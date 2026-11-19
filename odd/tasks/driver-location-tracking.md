# Feature: driver-location-tracking

Branch: `feat/tracking-position-domain`, base `develop`. The routing stack and the shared OSRM
module (#42 to #48) are already merged to `develop`.

## Objective

Receive the driver's GPS position while the driver is on a route, keep the latest position,
and estimate the road distance and driving time from that position to a delivery point using
the OSRM `Estimator`. This is the data order tracking needs for "where is my driver and how
long until arrival". The live customer view is out of this slice.

## Scope

- In:
  - New module `services/tracking-service` (hexagonal layout like `routing-service`).
  - Domain: `DriverPosition` (lat, lon, recorded_at), freshness rule, validation.
  - Port `PositionStore` with a Redis adapter (latest position per driver, TTL).
  - Use cases: `RecordPosition` and `EstimateArrival(driver, destination)`.
  - gRPC server `TrackingService` (internal only, not published to the host).
  - Gateway endpoint `PUT /api/v1/drivers/me/location`, JWT role `DRIVER`, resolves the driver
    with `GetDriverByUserID` and forwards the position to Tracking.
  - Redis wiring in `docker-compose.yml` for tracking (`REDIS_PASSWORD` already exists).
- Out:
  - Order status transitions `PICKED_UP`, `IN_TRANSIT`, `DELIVERED` (Sprint 5, owned by Anthony).
  - Public tracking link `GET /api/v1/tracking/{token}` and token handling (ADR 0004).
  - WebSocket / live push to the browser, frontend views.
  - Position history, route geometry (`overview=false` stays), caching of estimates.
  - Other regions than Ecuador.

## Constraints

- TDD strict (`make test` = `go test -race ./...` per module). Behavior tasks: RED then GREEN.
- Mirror `routing-service` layout, module `github.com/ErickGuerron/Backend-Pikka/services/tracking-service`,
  register in `go.work` and CI like the other modules.
- Position data is personal location data: never logged, never returned to customers in this slice.
- Redis key namespace `tracking:driver:{driver_id}`; values are JSON, TTL 120 s.
- gRPC contract goes in `contracts/proto/tracking/v1/tracking.proto` (the `.gitkeep` is already there);
  generated code in `contracts/gen/go/tracking/v1`.
- Unit tests with fakes; integration tests behind the `integration` tag and `REDIS_URL` /
  `OSRM_URL`, run against the real containers.
- Commits: Conventional Commits, Spanish, no AI attribution. No commit or push without the user asking.

## Design assumptions

These are decisions the docs do not settle. Flagged for review.

1. **Owner of the position store is Tracking Service.** Driver Service keeps the operational
   status; Tracking keeps the GPS stream. Tracking does not write to the driver database.
2. **Ingestion path.** The driver app calls the Gateway, which checks role `DRIVER`, resolves the
   driver id and calls Tracking over gRPC. Clients never call Tracking directly.
3. **Freshness.** A position older than 120 s is treated as stale (no ETA, explicit flag).
   Redis TTL matches it.
4. **Ordering.** A position with `recorded_at` older than the stored one is discarded; the
   stored one stays. Future timestamps beyond 30 s of server clock are rejected.
5. **Status check not enforced here.** `GetDriverByUserID` returns only id and user id, so the
   ON_ROUTE check is not possible in this slice. Any driver with a valid token may report.
   Revisit when the route-ownership check exists (Sprint 5).
6. **ETA reuses the OSRM adapter contract.** Same request shape (`lon,lat;lon,lat`,
   `radiuses=1000;1000`, `NoSegment`/`NoRoute` to `ErrNoRoute`). See open question A.
7. **Discarded fixes return 204.** The gateway answers 204 for an accepted fix and for a fix
   Tracking discarded as out of order, because the Tracking contract reports a discard as
   `accepted = false` with no error. Surfacing it (for example 409) would make the driver app
   handle a case it cannot act on. Flagged for review.
8. **No caching of the driver lookup.** Each report calls `GetDriverByUserID` once. Simple and
   correct for the MVP fix rate; revisit if the rate grows.
9. **Tracking does not `depends_on` OSRM.** The `osrm` service is profile-gated (`profiles: [osrm]`)
   and Compose rejects a `depends_on` on a disabled profile (see Progress, T8). Without OSRM,
   `EstimateArrival` returns INTERNAL, never a fabricated estimate.
10. **Gateway gets `DRIVER_SERVICE_ADDR` in Compose.** It was missing; the default
    `localhost:9092` is wrong inside the network. The new endpoint depends on the driver lookup.

## Decision: shared OSRM module (answer to open question A)

The user chose Option 2. The OSRM client is extracted to a reusable module, so the code is not
duplicated and each service stays independent of the other's domain.

- New module `libs/osrm` (module `github.com/ErickGuerron/Backend-Pikka/libs/osrm`), stdlib only.
  It owns its own `Point`, `Estimate`, `ErrInvalidPoint`, `ErrNoRoute`, and `Estimator`. It imports
  no service domain, so neither service depends on the other.
- `routing-service/internal/adapters/osrm` becomes a thin mapper: converts its domain `Point` to
  the shared one, and maps the shared errors back to its domain errors with `errors.Is`.
- `tracking-service` consumes the same module directly.
- Pure refactor: the existing OSRM tests move with the code and are the characterization. No new
  behavior, so no RED step; the moved tests must pass unchanged.

## Delivery split (answer to point 2)

Work units, each one reviewable PR, stacked in this order:

- PR 0 `refactor/osrm-shared-module`: create `libs/osrm`, move the estimator and its tests,
  thin mapper in routing-service, `go.work`, Makefile `osrm-test` path, CI matrix. Base: `feat/osrm-ecuador`.
- PR A `feat/tracking-position-domain`: T1 + T2 + T3 (contract, domain, application).
- PR B `feat/tracking-position-infra`: T4 + T5 + T6 (Redis adapter, OSRM via `libs/osrm`, gRPC server).
- PR C `feat/tracking-position-gateway`: T7 + T8 + T9 (gateway endpoint, compose, CI, lint).

No commits are made until the user asks for them. Each PR is measured with `git diff --shortstat`
against its base once committed.

## Tasks

- [x] T0 Shared OSRM module (PR 0, see above). Merged to `develop` (#48). Route: inline for the
      mapper; one writer for the module move. Check: `go test -race ./...` in `libs/osrm` and
      `services/routing-service`; `go test -tags integration -race ./...` in `libs/osrm` with `OSRM_URL`.
- [x] T1 Contract: `contracts/proto/tracking/v1/tracking.proto` (`RecordPosition`,
      `GetDriverPosition`, `EstimateArrival`), regenerate code, `buf lint`.
      Committed in `c20e9ec`. Route: inline (one contract file). Check: `buf lint` ok; `go build ./...` in `contracts`.
      Note: the proto declares `RecordPosition` and `EstimateArrival`; `GetDriverPosition` is not in the contract.
- [x] T2 Domain (TDD): `DriverPosition` validation, freshness, ordering rule. RED: undefined
      symbols; GREEN: table tests for range, NaN, future timestamp, stale, out-of-order.
      Committed in `316b31b`. Route: delegated writer (2+ non-trivial files). Check: `go test -race ./internal/domain/...`.
- [x] T3 Application (TDD): `RecordPosition` and `EstimateArrival` with fake `PositionStore`
      and fake `Estimator`. Stale or missing position returns a typed error, never an estimate.
      Committed in `316b31b`. Route: same writer, sequential after T2. Check: `go test -race ./internal/application/...`.
- [x] T4 Redis adapter (TDD): `PositionStore` on Redis with TTL and JSON; integration test
      against the real Redis container. Committed in `99fd811`. Route: delegated writer. Check: unit ok;
      `go test -tags integration -race ./internal/adapters/redis/...` with `REDIS_URL`.
- [x] T5 OSRM in Tracking: thin mapper over `libs/osrm` (no copy). TDD for the mapping.
      Committed in `99fd811`. Route: delegated writer. Check: unit ok; integration against `OSRM_URL` (Quito route).
- [x] T6 gRPC server (committed in `bb89bbd`) + `cmd/api/main.go` wiring, config from env
      (`REDIS_*`, `OSRM_URL`, `TRACKING_GRPC_PORT`), uncommitted. Route: delegated writer. Check: `go build ./cmd/...`;
      grpc server test with `bufconn`.
- [x] T7 Gateway: `PUT /api/v1/drivers/me/location` with JWT role DRIVER, validation,
      error mapping, no position in logs. Uncommitted. Route: delegated writer. Check: gateway handler
      tests; the endpoint is in `openapi.yaml`.
- [x] T8 Compose and CI: `tracking-service` service in `docker-compose.yml` (no published
      port), Redis env, `go.work` and `ci.yml` entries. Uncommitted. Route: inline. Check:
      `docker compose config` valid. `make up` not run: Docker daemon was not running.
- [ ] T9 Lint and full run: `golangci-lint` 0 issues, `go vet`, `gofmt`, `make test` for the
      module set. Route: inline. Check: all green, evidence recorded below. Partial evidence only
      (see Progress); not checked.

## Acceptance criteria

- A DRIVER token can `PUT /api/v1/drivers/me/location` with a valid position and gets `204`.
- An out-of-range position, a future timestamp, or an out-of-order one is rejected or
  discarded, with a stable error code.
- `EstimateArrival` for a driver with a fresh position returns distance and duration from OSRM.
- For a driver with no fresh position it returns a stale/missing result, never a fabricated estimate.
- Positions expire from Redis after 120 s.
- Position values do not appear in logs (checked by test on the handler and the use case).
- `make test` passes for the touched modules with `-race`; integration tests run, not SKIP.

## Delivery

- Size forecast: about 600-800 authored lines across the tracking tasks, above the 400-line
  budget, so it is split as described in "Delivery split" above.
- Base chain: `develop` -> PR A -> PR B -> PR C (PR 0 is merged).
- Current branch carries T1 to T8 uncommitted on top of the four tracking commits.

## Progress / evidence

- T0 / PR 0 (shared OSRM module), merged to `develop` (#48):
  - RED: after moving the estimator, `routing-service/internal/adapters/osrm` failed to build
    (`undefined: NewEstimator`) with the new mapper tests present.
  - GREEN: `libs/osrm` unit tests `-race` ok (9 tests moved unchanged); integration against
    real OSRM v5.25.0 (`OSRM_URL=http://127.0.0.1:5000`): Quito route and Bogota `ErrNoRoute` pass;
    `routing-service` all packages `-race` ok, 3 mapper tests pass.
  - Lint: golangci-lint 0 issues in `libs/osrm` and `routing-service`; `go vet` and `gofmt` clean.
  - Wiring: `go.work` (`./libs/osrm`), `services/routing-service/go.mod` (require + replace),
    `Makefile` `osrm-test` now runs in `libs/osrm`, CI matrix includes `libs/osrm`.
  - Files: `libs/osrm/{go.mod,estimator.go,estimator_test.go,estimator_integration_test.go}`
    (moved with `git mv`), `services/routing-service/internal/adapters/osrm/estimator.go` (mapper)
    and `estimator_test.go` (mapper tests).
- Mapping (inline, delegated explorer unavailable): driver-service has `ON_ROUTE` and
  `DRIVER_ARRIVAL_RADIUS_METERS`; `GeoPoint` and Haversine exist in driver domain;
  `drivers.proto` has no position RPC; gateway comment assigns "transiciones" to Tracking.
- Docker: OSRM, Redis and Postgres healthy; no tracking container exists.
- OSRM 400 check (previous step): the only 400 in the OSRM log is the Bogota `NoSegment` case.

### Status before this session (T2 to T5, committed on `feat/tracking-position-domain`)

- T2 domain (TDD, RED then GREEN): `NewPosition` validation, 120 s fresh window, 30 s clock-skew
  limit, strictly-newer ordering (`Supersedes`). `internal/domain/{position,estimate}.go`.
- T3 application (TDD, fakes): `RecordPosition` (discards out-of-order fixes, stores with TTL
  120 s) and `EstimateArrival` (no position -> `ErrNoPosition`, stale -> `ErrStalePosition`, never
  estimates from stale data). `internal/application/position/tracker.go`.
- T4 Redis adapter (TDD with miniredis; integration against the compose Redis): key
  `tracking:driver:{id}`, JSON value, TTL. Integration passed with `-tags integration`.
- T5 OSRM mapper over `libs/osrm` (TDD with httptest): NoSegment -> `domain.ErrNoRoute`.
- Lint: golangci-lint v2.14.0 0 issues; `go vet` and `gofmt` clean; `go test -race` ok.
- Wiring: `go.work` includes `./services/tracking-service`; `Makefile` GO_MODULES and CI
  matrix include `services/tracking-service`; `go mod tidy` run.
- Known limitation: `RecordPosition` checks the stored fix and then saves (not atomic). Two fixes
  for the same driver arriving at the same instant can race. Acceptable for MVP; a Lua
  compare-and-set can close it later.

### Session: T1, T6, T7, T8 (uncommitted, working tree)

Commits present: `316b31b` (T2, T3), `99fd811` (T4, T5), `c20e9ec` (T1), `bb89bbd` (T6 gRPC server).

- T1 (checked in `c20e9ec`): `buf lint` exit 0; `go build ./...` in `contracts` exit 0;
  `buf generate` then `git diff --exit-code -- contracts/gen` exit 0 (no drift).
- T6 main wiring (uncommitted):
  - RED: `go test ./internal/infrastructure/config/` -> `undefined: Load` (build failed).
  - GREEN: `go test -race ./internal/infrastructure/config/` ok; `go build ./cmd/...` exit 0.
  - Files: `cmd/api/main.go`, `internal/infrastructure/config/{config.go,config_test.go}`.
  - Config: `TRACKING_GRPC_PORT` (default 9093), `REDIS_ADDR` (default `localhost:6379`),
    `REDIS_PASSWORD` (required), `OSRM_URL` (http or https, default `http://localhost:5000`).
  - No HTTP listener: the spec gives tracking only a gRPC port.
  - The driver-service `main.go` was the pattern. `routing-service` has no `cmd/api/main.go`
    or Dockerfile, so it was not usable as the pattern.
- T7 gateway endpoint (uncommitted):
  - RED: `go test ./internal/adapters/http/ ./internal/infrastructure/config/` -> build failed:
    `undefined: httpapi.TrackingClient`, `unknown field Tracking in struct literal`,
    `cfg.TrackingServiceAddr undefined`.
  - GREEN: `go test -race ./...` in `services/api-gateway` ok, all packages. Cases:
    401 without token; 403 for ADMIN, OPERATOR, CUSTOMER with zero internal calls; 204 with the
    position forwarded; 400 table (malformed JSON, missing fields, bad RFC 3339, string latitude)
    with zero internal calls; 415 wrong content type; Tracking error mapping (InvalidArgument 400,
    Unavailable 503, Internal 500); unknown driver 404 with no Tracking call; logs contain
    no coordinates on the success and error paths (the logger is injected, so the test reads real output).
  - Files: `internal/adapters/http/drivers_location_handlers.go` (+ `_test.go`),
    `internal/adapters/http/router.go` (`Options.Tracking`, PUT route in the DRIVER group),
    `internal/adapters/grpcclient/tracking.go`, `internal/infrastructure/config/{config.go,config_test.go}`
    (`TRACKING_GRPC_ADDR`, default `localhost:9093`), `cmd/api/main.go`.
  - `contracts/openapi/openapi.yaml`: `PUT /api/v1/drivers/me/location`, `DriverLocationRequest`
    schema, `drivers` tag description updated.
  - No unit test for the `grpcclient/tracking.go` adapter (thin wrapper, same as Drivers).
- T8 Compose, Dockerfile, CI (uncommitted):
  - `services/tracking-service/Dockerfile`: same shape as driver-service, plus `libs/osrm` in the
    build context (the module has a local replace). The build steps were reproduced in a clean temp
    tree with `GOWORK=off` (`go mod download` and `go build -trimpath ./cmd/api`), both exit 0.
    The image itself was not built (see below).
  - `docker-compose.yml`: `tracking-service` (no `ports`, `REDIS_ADDR: redis:6379`,
    `REDIS_PASSWORD`, `OSRM_URL: http://osrm:5000`, `TRACKING_GRPC_PORT: "9093"`, depends on redis
    healthy). Gateway gets `TRACKING_GRPC_ADDR: tracking-service:9093`, `DRIVER_SERVICE_ADDR:
    driver-service:9092` (was missing), and depends on driver-service and tracking-service.
  - `.github/workflows/ci.yml`: docker job builds `services/tracking-service/Dockerfile`.
  - `go.work` and the CI matrix already included `services/tracking-service` (verified).
  - Checks: `docker compose --env-file .env.example config --quiet` exit 0;
    `docker compose --env-file .env.example --profile osrm config --quiet` exit 0; ci.yml and
    docker-compose.yml parse as YAML.
  - Negative check: a temporary compose with `depends_on: osrm` (profile off) failed with
    `service "tracking-service" depends on undefined service "osrm": invalid compose project`
    (exit 1). That is why the dependency was not added (design assumption 9).
  - Docker daemon not running (`failed to connect to the docker API ... dockerDesktopLinuxEngine`):
    `docker build` and `make up` were NOT run.

### Verification run (this session)

- `services/tracking-service`: `go test -race -count=1 ./...` all ok; `go build ./cmd/...` exit 0;
  `go vet ./...` exit 0; `gofmt -l .` empty; `golangci-lint run --config ../../.golangci.yml ./...` 0 issues.
- `services/api-gateway`: `go test -race -count=1 ./...` all ok; `go vet ./...` exit 0;
  `gofmt -l .` empty; `golangci-lint run --config ../../.golangci.yml ./...` 0 issues.
- `contracts`: `go build ./...` exit 0; `buf lint` exit 0; `buf generate` no drift.
- `openapi-lint` (`npx @redocly/cli@1.34.0 lint`): valid, 0 errors. 3 warnings were already there on
  the 501 driver routes (`operation-2xx-response`); the new operation has no warning.
- Not run this session: Redis and OSRM integration tests (`-tags integration`); Docker is not running.
  The earlier recorded results for T4 and T5 still stand, but this session did not re-run them.
- T9 is not checked: no `make test` over the whole module set, no `make lint`, no CI run.

## Next step

Ask the user to approve the commit split (PR B: T4, T5, T6; PR C: T7, T8). Then run the Redis and
OSRM integration tests when Docker is up, `docker compose up` for the healthy check, and T9.
Frontend (live route and tracking view) starts after T7 is committed.
