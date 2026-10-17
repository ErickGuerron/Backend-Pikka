# Feature: routing-service-runtime (R1)

Branch: `feat/routing-service-runtime`, base `feat/tracking-stop-status` (uncommitted work carried over). Part R1 of three. R2 (Gateway route) and R3 (Driver Service) are not done here.

## Objective

Make the Routing Service a running process. It serves `routing.v1.GeneratePlan`, which plans the PENDING orders of a delivery date, reserves one driver per route in Driver Service, and then assigns the route's orders in Order Service. The reservation closes a gap: without a reserved driver, `GetDriverByRouteID` (Driver Service) cannot find the driver that tracking needs.

## Scope

- In:
  - `contracts/proto/routing/v1/routing.proto`: `RoutingService.GeneratePlan`.
  - `plan.Generate`: reserve the driver of each route, then assign its orders; stop rules; result per route.
  - `ports.DriverPool` and its adapter in `adapters/clients` (Driver Service `FindAvailableDrivers`, `ReserveDriver`).
  - gRPC `RoutingServer` with the role gate (ADMIN, OPERATOR).
  - `infrastructure/config` (env), `cmd/api/main.go`, `Dockerfile`.
- Out:
  - No routing database and no migrations (the routing service is stateless).
  - Gateway route `POST /api/v1/route-plans/generate` (R2).
  - Driver Service changes, including releasing a reserved driver (R3).
  - Compose service entry and README update.

## Constraints

- Strict TDD: RED then GREEN, evidence below.
- English identifiers and comments. Stdlib and existing dependencies only (no new modules).
- Generated code: `buf generate --path proto/routing/v1/routing.proto` (buf 1.50.0), run in `contracts/`.
- Conventional Commits. No commit, push or PR in this change (user instruction).

## Design assumptions

- No database. Routing state lives in Order Service (`route_id`, stops) and Driver Service (reservations).
- Order of operations per route: reserve the driver, then assign the orders. A route never gets orders without a driver.
- `FindAvailable(ctx, 1)` is called once per route, so each route sees the drivers reserved before it. Simpler than one up-front list and equally correct.
- Stop rule: the first route without a driver, or with a failed reserve or assign, stops the run. Earlier routes are kept (not atomic across routes); the failing route's orders are untouched.
- Known gap: if the assign fails after a reserve succeeded, the driver stays ASSIGNED to a route with no orders. Driver Service has no release RPC yet (R3 or later).
- Role: `x-user-role` must be ADMIN or OPERATOR (ADR 0008). Blank or missing role is UNAUTHENTICATED; other roles are PERMISSION_DENIED before any downstream call.
- Config: `ROUTING_GRPC_PORT` (default 9094); service addresses default to the Gateway's local values (`localhost:9090` zone, `localhost:9091` order, `localhost:9092` driver).
- Only `routing/v1` was regenerated. Other `contracts/gen/go` diffs were already in the working tree.

## Tasks

- [x] T1 Port `DriverPool`, `plan.ErrNoDriverAvailable`, `Result.Routes`, `New(..., drivers, ...)`. Route: inline. Check: plan and clients tests (RED below).
- [x] T2 `Generate.Run`: reserve then assign, stop rules, per-route result. Route: inline. Check: plan tests GREEN.
- [x] T3 `clients.DriverPool` adapter, identity forwarded. Route: inline. Check: clients tests GREEN.
- [x] T4 `routing.proto` with status codes documented; `buf generate` and `buf lint`. Route: inline. RED not possible: contract-only, no behavior. Check: `buf lint` exit 0.
- [x] T5 gRPC `RoutingServer`: role gate, error mapping, success mapping. Route: inline. Check: grpc tests GREEN.
- [x] T6 `infrastructure/config`: env with defaults, port range. Route: inline. Check: config tests GREEN.
- [x] T7 `cmd/api/main.go` (dial, wire, graceful stop) and `Dockerfile`. Route: inline. RED not possible: wiring only; verified by `go build` and `docker build`.
- [ ] T8 Gateway route for generation (R2). Not done here.
- [ ] T9 Driver Service release of a reserved driver for failed routes (R3 or later). Not done here.

## Acceptance criteria

- `go test -race -count=1 ./...` passes in `services/routing-service`.
- `go vet ./...` and `gofmt -l .` are clean.
- A route is never assigned without its driver reserved first (test: reservation order trace).
- With no available driver, or a failed reserve, no order of the failing route changes (tests).
- Role gate runs before any downstream call (test: fake use case call count is 0).
- `buf lint` passes in `contracts/`; `go build ./...` passes in `contracts/` and `services/api-gateway/`.
- The routing image builds from the repository root.

## Delivery

- Size: 1316 lines against a 400-line budget (excluding `*.pb.go`). This is over budget; see Progress for the split.
- PR strategy: decided at R1 review, with the over-budget case in mind. No PR created (user instruction).

## Progress / evidence

RED (before implementation):

- `go test ./internal/application/plan/` (behavioral RED, after the scaffolding of T1 and before T2): 7 failures, for example
  `generate_test.go:270: steps = [assign 2026-10-15/A#1 assign 2026-10-15/A#2], want [reserve d1 2026-10-15/A#1 assign ...]` and `generate_test.go:290: err = <nil>, want ErrNoDriverAvailable`.
- `go test ./internal/adapters/clients/` (before T3): build failed, `undefined: NewDriverPool`.
- `go test ./internal/adapters/grpc/ ./internal/infrastructure/config/` (before T5, T6): setup failed, `routing/v1` package not found; `undefined: Load`.

GREEN (after T2 to T6):

- `go test -race -count=1 ./...`: all packages `ok` (clients, grpc, osrm, plan, domain, config); `cmd/api` and `ports` have no test files.
- `go vet ./...`: exit 0. `gofmt -l .`: no output. `go build ./cmd/api`: exit 0.
- `buf generate --path proto/routing/v1/routing.proto` exit 0; `buf lint` exit 0 (no output).
- `go build ./...` in `contracts/`: exit 0; `go build ./...` in `services/api-gateway/`: exit 0.
- `docker build -f services/routing-service/Dockerfile .`: failed in `go mod download` (container DNS could not resolve proxy.golang.org; sandbox network). `docker build --network host ...`: exit 0.

Authored line count (excluding `*.pb.go`, whole final files): 504 in new files (proto, server, config, main, Dockerfile, both new test files, this document); 812 in modified shared files (`clients.go`, `clients_test.go`, `generate.go`, `generate_test.go`, `planning.go`), which already held earlier uncommitted work. Total 1316, over the 400 budget. Flagged for review: split or accept at R1 review.

## R2 driver create and release

Scope: Driver Service `CreateDriver` (ADMIN only) and `ReleaseDriver`; the Routing Service releases the driver of a route whose assignment failed. The Gateway route that calls `CreateDriver` (role DRIVER check) is R3. Not done here.

Decisions:

- `ReleaseDriver` mirrors `ReserveDriver`, which has no role gate in Driver Service. The gate is the Gateway's job, and these RPCs are internal. Gap: an internal caller without a role is not refused; relies on network isolation.
- New domain method `Driver.ReleaseRoute` instead of changing `Release`: the existing `Release` (ASSIGNED only, `ErrInvalidTransition` on ON_ROUTE) is covered by current tests and keeps that contract.
- `ReleaseRoute` is idempotent: a driver with no reservation is a no-op. A different route returns `ErrRouteMismatch` (FAILED_PRECONDITION).
- `CreateDriver` generates a UUID v4 (stdlib `crypto/rand`, no new module). Duplicate user: migration `000005` adds a unique index on `drivers.user_id`; memory repo applies the same rule. Non-UUID `user_id` in PostgreSQL maps to `ErrInvalidUserID` (INVALID_ARGUMENT).
- Routing: on assign failure after a reserve, `Generate.Run` calls `Release` with a 5 s timeout, detached from request cancellation. A failed release is reported together with the assign error, and the message says the driver stays reserved.

RED (before implementation):

- `go test ./internal/domain/` (driver-service): build failed, `d.ReleaseRoute undefined`.
- `go test ./internal/application/driver/`: build failed, `svc.CreateDriver undefined`, `svc.ReleaseDriver undefined`.
- `go test ./internal/adapters/grpc/`: `CreateDriver` and `ReleaseDriver` tests FAIL (the RPCs answer Unimplemented from the embedded server).
- `go test ./internal/application/plan/` (routing): 4 FAIL: `TestGenerateReportsRoutesAssignedBeforeAFailure` (`released = []`), `TestGenerateReleasesDriverWhenAssignFails` (`released = []`, want d1 on A#1), `TestGenerateReleasesBeforeStoppingAfterAssignFails` (no release step), `TestGenerateReportsReleaseFailureWithAssignError` (release error not reported).
- `go test ./internal/adapters/clients/` (routing): 2 FAIL: `TestDriverPoolReleaseSendsDriverAndRouteAndForwardsIdentity` (`release sent = []`), `TestDriverPoolReleasePreservesDriverServiceErrorCode` (code = OK).

GREEN:

- driver-service `go test -race -count=1 ./...`: all packages `ok` (grpc, application/driver, domain, config).
- routing-service `go test -race -count=1 ./...`: all packages `ok` (clients, grpc, osrm, plan, domain, config).

Checks (exit codes and output as run):

- contracts: `buf generate --path proto/drivers/v1/drivers.proto` ok; `buf lint` exit 0; `go build ./...` exit 0.
- api-gateway: `go build ./...` exit 0.
- driver-service: `go vet ./...` exit 0; `go vet -tags integration ./...` exit 0; `gofmt -l .` no output.
- routing-service: `go vet ./...` exit 0; `gofmt -l .` no output.

Not done:

- PostgreSQL integration tests (`driver_create_release_integration_test.go`, changed `driver_repository_integration_test.go`) only compile with `-tags integration`. Not run: the shared DB is off limits. Migration `000005` fails on a database that already has two drivers with the same `user_id`.
- Gateway route for `CreateDriver` (R3) and the generation route (still pending from R1).
- No retry or outbox for the release: a failed release leaves the driver reserved, as reported.
- Size: R2 is over the 400-line budget (about 411 new lines plus about 330 in shared files). Split before review.
- `drivers.proto` has pre-existing encoding damage in some comments (for example `CA3digos`). Not touched in R2.

## R3 Gateway routes (plan generation and driver creation)

Scope: Gateway `POST /api/v1/routes/plan` (ADMIN, OPERATOR) over `routing.v1.GeneratePlan`; Gateway `POST /api/v1/drivers` (ADMIN) over `driver.v1.CreateDriver`, after checking the user's DRIVER role with Auth `GetUser`; `ROUTING_SERVICE_ADDR` config; compose service; CI image build; OpenAPI. Not done: live run (R4).

Decisions:

- Path `/api/v1/routes/plan` as requested for R3. The earlier planned path `/api/v1/route-plans/generate` is superseded and not used anywhere.
- Plan gate: existing `StaffOnly` (ADMIN and OPERATOR, already used by the catalog). No new middleware.
- Driver create: the handler calls Auth `GetUser`. Missing user: Auth NOT_FOUND, returned as 404. Existing user without DRIVER role: `FAILED_PRECONDITION`, returned as 422. Chosen over 400 because the body is valid and the user's state blocks the action.
- ALREADY_EXISTS -> 409: the existing `translate` table already maps `codes.AlreadyExists` to 409 CONFLICT. `errors.go` is unchanged; the mapping is covered by `TestCreateDriver_DriverServiceErrors_MapToErrorEnvelope`.
- New narrow interfaces `RoutingClient` and `DriverCreator`, separate from `DriverClient`, so no existing fake changes. The driver connection is reused for `CreateDriver`; the routing connection is new.
- `ROUTING_DB_PASSWORD`: KEPT in `docker-compose.yml`, `.env.example` and CI. Routing has no database in code, but `infrastructure/postgres/init/01-schemas-and-users.sh` (line 32) provisions `routing_service_user` from it and aborts with `missing password` when it is empty. Removing it from compose would break Postgres init. Removing it properly means editing that script too: a separate change.
- routing-service in compose: no published port, no `depends_on` on postgres, `depends_on` order, zone and driver. `JWT_SECRET` arrives through the shared `go-env` anchor, as for the other Go services; routing does not read it.
- Already in place, no change: `routing-service` is in `go.work`, in `GO_MODULES` (Makefile) and in the CI test matrix.
- Known gap, not in spec: the driver create does not check that the user is active. A deactivated DRIVER user can get a profile.

RED (before implementation, behavioural):

- Scaffolding first (fields and interfaces only, no routes and no handlers), then the tests.
- `go test -count=1 -run 'RoutePlan|CreateDriver|TestLoad_RoutingServiceAddr' ./internal/adapters/http/ ./internal/infrastructure/config/` in `services/api-gateway`: 10 top-level tests and 21 subtests FAIL in `http` (new-route expectations of 200, 400, 415, 422, 409, 503 receive 404). `TestLoad_RoutingServiceAddr` FAILs 3 subtests (`""` instead of `localhost:9094`).
- Also in RED, the 401 and 403 role tests and the unknown-user 404 test PASS, for reasons unrelated to the new routes. Unregistered `/api/v1/*` paths are answered by the group middleware catch-all (`AdminOnly` in this router). They stay as regression guards; the RED evidence is the failing list above.
- Before RED, one build failure: helper name `bearer` already declared in `zones_routes_test.go`. Renamed to `authHeaderFor` in the new tests only.

GREEN:

- `go test -race -count=1 ./...` in `services/api-gateway`: all packages `ok` (grpcclient, http, config, jwt).
- `go vet ./...`: exit 0. `gofmt -l .`: no output. `go build ./...`: exit 0.
- `golangci-lint run --config ../../.golangci.yml ./...` (v2.14.0, the CI version): 7 issues (bodyclose 3, errcheck 3, unused 1), all in pre-existing untracked files `tracking_ws.go`, `tracking_ws_test.go`, `tracking_snapshot_test.go`. None in R3 files. Not fixed: outside this change.
- `services/driver-service`: `go build ./...` exit 0. `services/routing-service`: `go build ./cmd/...` exit 0.
- `docker compose --env-file .env.example config --quiet`: exit 0. `routing-service`: 0 published ports, depends on order, zone and driver. The gateway still publishes only 8080.
- `docker build --network host -f services/routing-service/Dockerfile .`: exit 0. Without `--network host` the sandbox DNS fails, as in R1.
- OpenAPI `npx @redocly/cli@1.34.0 lint --config contracts/openapi/redocly.yaml contracts/openapi/openapi.yaml`: valid, 0 errors. Warnings: 3 before this change and 3 after (the same pre-existing ones: tracking WebSocket 101, `drivers/me/route`, `drivers/me/.../result`). Baseline taken on the file before the R3 edits, not on a stash-free copy.

Authored lines (excluding `*.pb.go`):

- New Go files: 517 (176 production, 341 tests). Over the 400-line budget. Suggested split: production files first, tests with them.
- Shared files (`router.go`, `main.go`, `config.go`, `config_test.go`, `grpcclient/drivers.go`, `openapi.yaml`, `docker-compose.yml`, `ci.yml`): R3 hunks only, about 260 lines (estimate). These files already held R1/R2 uncommitted work, so `git diff --numstat` cannot separate them. `docker-compose.yml` (+20) and `ci.yml` (+2) had no earlier changes, so those counts are exact.

Not verified (left for R4 or later):

- Live run: gateway to routing to driver, order and zone over the compose network. Not started. Plan and create are only exercised with fakes at the gateway boundary.
- `FAILED_PRECONDITION` from a real Driver Service reservation: not exercised end to end.
- Inactive users can receive a driver profile (see known gap).
- No README update for the gateway routes.

## Cleanup

- Lint fix in `services/api-gateway` (the 7 issues above): `tracking_ws_test.go` now closes `resp.Body` after each dial that returns a response, guarded by `resp != nil && resp.Body != nil` (coder/websocket v1.8.15 sets `resp.Body = nil` on a successful upgrade). `defer conn.CloseNow()` is wrapped as `defer func() { _ = conn.CloseNow() }()` (best-effort close) in `tracking_ws.go` and `tracking_ws_test.go`. Unused `reasonPtr` removed from `tracking_snapshot_test.go` (no references). Test behavior unchanged.
- Active-user check on `POST /api/v1/drivers` (`driver_admin_handlers.go`): after the DRIVER role check, a user with `active=false` (auth `User.active`, field 5) gets 422 `FAILED_PRECONDITION` and no `CreateDriver` call. Chosen to match the wrong-role mapping. TDD: `TestCreateDriver_InactiveDriverUser_Returns422WithoutCreating` failed first (201), then passed. This closes the known gap noted above.
- Commands (api-gateway):
  - `golangci-lint run ./...`: `0 issues.` (exit 0)
  - `go test -race -count=1 ./...`: ok for grpcclient, http, config, jwt
  - `go vet ./...`: exit 0. `gofmt -l .`: no output.

## Fix: gRPC reconnect

Bug: after docker compose recreates an upstream gRPC service, the gateway keeps failing with `connection refused` against the old container IP for minutes, until it is restarted.

Cause (grpc-go `v1.85.0-dev.0.20260825072537-93e31b48545e`):

- DNS re-resolution is throttled: `internal/resolver/dns/dns_resolver.go:50-52` sets `MinResolutionInterval = 30 * time.Second` (internal, not configurable through `grpc.Dial` options).
- Connect backoff default grows up to 120s: `backoff/backoff.go:51` (`DefaultConfig.MaxDelay: 120 * time.Second`).

Fix: `services/api-gateway/internal/adapters/grpcclient/auth.go`. `Dial(addr)` adds `grpc.WithConnectParams(reconnectParams)`, with `Backoff{BaseDelay: 500ms, Multiplier: 1.6, Jitter: 0.2, MaxDelay: 5s}` and `MinConnectTimeout: 5s`. It is a package-level variable, not a `const`, because a struct literal cannot be a Go constant. The comment explains the stale DNS case. No other behavior changed.

RED (before the fix, default backoff), `go test -count=1 -run TestDialReconnects ./internal/adapters/grpcclient/ -v`:

- Downtime 12s: PASS, reconnected 4.49s after restart. The old backoff's next retry was inside the 8s bound, so 12s did not expose the bug.
- Downtime 17s: FAIL, `connection state is TRANSIENT_FAILURE after 8s; want READY` (25.03s). Reproduced.

Test design note: after a READY connection drops, the channel goes to IDLE. The first RPC (here, periodic health probes during downtime, like gateway traffic) starts the failing attempts that enter TRANSIENT_FAILURE and follow the backoff. A first version that called `Connect()` from the wait loop while IDLE reconnected at once and passed for the wrong reason, so the loop does not do that.

GREEN (with the fix), same test, 17s downtime: PASS, `reconnected 151ms after upstream restart` (17.19s).

Test: `internal/adapters/grpcclient/dial_test.go`, `TestDialReconnects`. Real gRPC health server on a free 127.0.0.1 port, restarted on the same address. Skipped under `-short`. Duration about 17s with 17s downtime.

Not verified:

- The real container IP change under docker compose. I could not reproduce the exact IP change; the test restarts a server on the same address. The bound (about 5s max backoff, so reconnect within 8s) is validated by the test, not by a live compose run.
- The 30s DNS interval is not exercised: the test uses an IP address target, which bypasses DNS. The fix bounds the backoff only; it does not change DNS re-resolution.
- The RED window depends on backoff jitter; the 17s value was chosen from the default sequence, and only one RED run was recorded.

## Next step

R4: live run of the stack (`docker compose up` for the services in this change), a plan generation through the gateway with real orders, zones and drivers, and the release path of a failed route. Decide the active-user check for driver create before R4.
