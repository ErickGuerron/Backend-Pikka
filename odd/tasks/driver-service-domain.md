# Feature: driver-service-domain

Branch: `feat/driver-service-domain` (from `main` @ 6417088)

## Objective

Sprint deliverable: `drivers.v1` gRPC contract and the Driver Service domain
(entity and operational states) with unit tests. No persistence, transport or
Docker image yet.

## Scope

- In: `contracts/proto/drivers/v1/drivers.proto` + generated Go, new module
  `services/driver-service` (domain layer only), wiring in `go.work`, `Makefile`
  `GO_MODULES` and CI module matrix.
- Out: gRPC server, PostgreSQL adapter and migrations, Dockerfile, compose entry,
  gateway routes, OWASP `security-scan` (separate deliverable, no target exists).

## Constraints

- Mirror `services/auth-service` (hexagonal, own `go.mod`, module path
  `github.com/ErickGuerron/Backend-Pikka/services/driver-service`, go 1.26.0).
- Domain tests: package `domain_test`, testify `assert`/`require`, table-driven
  where natural. Sentinel errors in `errors.go`. English messages and identifiers.
- Proto conventions from `auth.v1`: `package drivers.v1`, `go_package` ending
  `drivers/v1;driversv1`, enum `*_UNSPECIFIED = 0`, unique request/response per
  rpc, timestamps as RFC3339 strings, Spanish comments.
- Source of truth: BASE_TECNICA.md sections 3.4 (L137-156), 8.6 (L527-536),
  gRPC example (L611-634), flow (L1373-1384).
- TDD: strict. Runner: `go test -race ./...` per module. Source: project
  configuration (Strict TDD Mode enabled). RED observed before implementation.
- Tools missing locally: `make`, `buf`, `protoc-gen-go(-grpc)`, `golangci-lint`.
  Run the underlying commands directly (`go test`, `go vet`, `gofmt`); install buf
  and protoc plugins with `go install` if needed for T1.
- Commits: Conventional Commits, no AI attribution. No push / PR without user.

## Design assumptions (not defined in the docs, flagged for user review)

States (doc 3.4): `AVAILABLE`, `ASSIGNED`, `ON_ROUTE`, `OFFLINE`.
Transitions:
- `AVAILABLE -> ASSIGNED` (route reserved), `AVAILABLE -> OFFLINE`
- `ASSIGNED -> ON_ROUTE` (route started), `ASSIGNED -> AVAILABLE` (route
  released/cancelled), `ASSIGNED -> OFFLINE`
- `ON_ROUTE -> AVAILABLE` (route completed). `ON_ROUTE -> OFFLINE` forbidden
  (a driver mid-route cannot go offline).
- `OFFLINE -> AVAILABLE`
- Same-state transitions rejected. A new driver starts AVAILABLE and needs a
  non-blank id (`ErrInvalidDriverID`).
- Inactive drivers: cannot receive a new reservation (`ErrDriverInactive`), cannot
  `StartRoute` (`ErrDriverInactive`) and cannot `GoOnline`; they can never end up
  AVAILABLE. A deactivated driver that holds a reservation can still `Release` or
  `CompleteRoute` (with the reserved route id), ending OFFLINE.
- `Reserve` order: validate route id, same-route retry while ASSIGNED/ON_ROUTE
  returns nil (idempotent, even if now inactive), other route while ASSIGNED/ON_ROUTE
  returns `ErrDriverAlreadyReserved` (also if inactive), inactive returns
  `ErrDriverInactive`, OFFLINE returns `ErrDriverOffline`, then the transition.
- `Release(routeID, now)`, `StartRoute(routeID, now)` and `CompleteRoute(routeID, now)`
  are scoped to the reserved route. Error order: blank route `ErrInvalidRouteID`;
  (`StartRoute` only) inactive `ErrDriverInactive`; wrong origin state
  (ASSIGNED for Release/StartRoute, ON_ROUTE for CompleteRoute)
  `ErrInvalidTransition`; other route `ErrRouteMismatch`. A failed call changes
  nothing.
- `GoOffline` from ASSIGNED drops the reservation (`RouteID` cleared). Allowed but
  documented: the application layer must tell the Routing Service so the order gets
  another driver.
Driver fields: id, userId (auth user with DRIVER role), fullName, phone, active,
status, routeId, version, createdAt, updatedAt.

## Tasks

- [x] T1 Contract `drivers.v1`: `drivers.proto` (DriverService: FindAvailableDrivers,
      ReserveDriver; Driver, DriverStatus) and generated code in `contracts/gen/go`.
      Route: delegated. Check: proto compiles / buf lint if installable.
- [x] T2 Module scaffold: `services/driver-service/go.mod`, `go.work`, Makefile
      `GO_MODULES`, CI matrix. Route: delegated (same writer). Check: `go build ./...`.
- [x] T3 Domain status + transitions (TDD): `status.go`, `errors.go`,
      `status_test.go`. Check: `go test -race ./...` RED then GREEN.
- [x] T4 Domain Driver entity (TDD): `driver.go`, validation, `Reserve`/`StartRoute`/
      `CompleteRoute`/`Release`/`GoOffline`/`GoOnline`, `driver_test.go`.
      Check: `go test -race ./...`, `go vet`, `gofmt -l`.

## Follow-up requested by the user after the first review (2026-10-09)

Decisions (user-owned business rules):
- A driver cannot be reserved twice: one person cannot serve two different orders
  in the same period. Same-route reserve is an idempotent retry (contract
  `ReserveDriver` is idempotent); a different route is rejected with a specific
  error. The driver now remembers its current route (`RouteID`).
- An inactive driver can never return to AVAILABLE (`GoOnline` checks `Active`).
- `NewDriver` starts AVAILABLE ("online"): a driver is registered and given work
  moments later. This replaces the earlier OFFLINE assumption.
- Real `-race` verification is required: install gcc so cgo works.

- [x] T5 Tooling: install gcc (scoop) so `go test -race` runs. Check: `go test -race ./...`
      in driver-service, auth-service and api-gateway.
      Evidence: scoop installed gcc 15.2.0; parent ran `go test -race -count=1 ./...`
      OK in driver-service, auth-service and api-gateway.
- [x] T6 Domain rules (TDD): `Reserve(routeID, now)` idempotent per route, new
      `ErrDriverAlreadyReserved`, `RouteID` cleared on release/complete/offline;
      `GoOnline` rejects inactive drivers; `NewDriver` starts AVAILABLE.
      Route: delegated writer (4 files: driver.go, errors.go, driver_test.go, doc).
      Check: `go test -race ./...`, `go vet`, `gofmt -l`.
      Evidence: commits 5869f00 (starts AVAILABLE), bf3ba15 (`Reserve(routeID, now)`,
      `ErrInvalidRouteID`, `ErrDriverAlreadyReserved`, `RouteID` cleared), cb5ad42
      (inactive drivers: `GoOnline` -> `ErrDriverInactive`; `Release`/`CompleteRoute`
      end OFFLINE and clear `RouteID`). RED: assertion AVAILABLE vs OFFLINE; compile
      failure (undefined `RouteID`, `Reserve` arity); `GoOnline`/`Release`/
      `CompleteRoute` inactive tests failing. GREEN: `go test -race` ok, 100% coverage.

- [x] T7 Review polish (TDD, native review findings): blank driver id, inactive
      `StartRoute`, idempotent same-route `Reserve` for inactive drivers, stale
      comments, error limits derived from constants.
      Route: delegated writer (one writer, sequential work-unit commits).
      Evidence:
      - b094925 `fix(driver-service): reject blank driver id`. RED: compile failure
        (undefined `ErrInvalidDriverID`), then with the sentinel added the new
        id tests failed (no validation, id not trimmed). GREEN after implementing.
      - a5bd4dd `fix(driver-service): block inactive drivers from starting routes`.
        RED: `TestDriverStartRouteInactivo` failed in all four states. GREEN.
      - 509ae84 `fix(driver-service): make same-route reserve retry idempotent for
        inactive drivers`. RED: idempotent and other-route tests failed for
        inactive ASSIGNED/ON_ROUTE. GREEN. Known limit documented in `Reserve`.
      - 1d92742 `refactor(driver-service): fix stale comments and derive error limits
        from constants` (no behavior change, tests green throughout; removed
        `guardedTransition`).
      - Final: `go test -race -count=1 -cover ./...` 100% statements, `go vet`,
        `gofmt -l` clean.

- [x] T8 Contract documentation: `drivers.proto` comments only (no wire change)
      document `ReserveDriver` (required fields, idempotency and its limit, one route
      at a time, ASSIGNED on success), the gRPC status table the server must
      implement (INVALID_ARGUMENT, NOT_FOUND, FAILED_PRECONDITION) and
      `FindAvailableDrivers` (active and AVAILABLE only).
      Evidence: eaa033c `docs(contracts): document ReserveDriver semantics and error
      mapping`. `buf lint` clean; `buf generate` only touched
      `contracts/gen/go/drivers/v1` (comments), auth files unchanged;
      `buf breaking contracts --against ".git#branch=main,subdir=contracts"` (CI
      form) exit 0; `go build ./...` in contracts ok.
- [x] T9 Minor review details (no behavior change, no RED possible): `NormalizePhone`
      comment states it is a simplified check (no E.164 or country-code validation),
      `GoOnline` explains why `ErrDriverInactive` precedes `ErrInvalidTransition`,
      test constant `maxFullNameRunesPlusOne` replaces the magic 121. No Spanish
      non-test identifiers were found in the tests, so no renames were needed.
      Evidence: 720a6d5 `refactor(driver-service): clarify comments and test names`.
      `go test -race -count=1 -cover ./...` 100.0%, `go vet`, `gofmt -l` clean;
      auth-service and api-gateway `-race` tests ok.

- [x] T10 Domain (TDD): route-scoped `Release`/`StartRoute`/`CompleteRoute`
      (`ErrRouteMismatch`), `Reserve` on an OFFLINE driver returns `ErrDriverOffline`,
      `GoOffline` doc comment about dropping the reservation. Route: delegated writer.
      Evidence: a3470c1 `feat(driver-service): scope route transitions to the reserved
      route`. RED: compile failure (undefined `domain.ErrDriverOffline`, methods
      called with an extra route argument). GREEN: `go test -race` ok, 100% coverage
      (an unreachable `transition` error branch in `Reserve` was replaced by an
      explicit `StatusAvailable` guard covered by an unknown-status test). Tests cover
      every precedence step and success path for active and inactive drivers.
- [x] T11 Contract: `DriverSummary {id, user_id}` replaces `Driver` in
      `FindAvailableDriversResponse` (data minimization; `ReserveDriverResponse` keeps
      `Driver`), `limit` documented (0 = default 50, max 100 capped, negative is
      INVALID_ARGUMENT) and `ReserveDriver` doc covers OFFLINE explicitly.
      Evidence: 6415cd7 `feat(contracts): return driver summaries and bound the
      available drivers limit`. `buf lint` clean; `buf generate` only touched
      `contracts/gen/go/drivers/v1`; `buf breaking contracts --against
      ".git#branch=main,subdir=contracts"` exit 0 (`drivers/v1` is not in `main`);
      `go build ./...` in contracts ok. No RED (contract/doc change).
- [x] T12 Cleanup (no behavior change, tests green throughout): validation limits
      exported as `MaxFullNameRunes`, `MinPhoneDigits`, `MaxPhoneDigits` and used by
      errors and tests (removed `maxFullNameRunesPlusOne`). No Spanish non-`Test*`
      identifiers exist in `driver_test.go` or `status_test.go` (the reviewer's note
      referred to `Test*` names, which stay Spanish), so no renames.
      Evidence: 954e60e `refactor(driver-service): remove duplicated limits and
      non-English test identifiers`. `go test -race -count=1 -cover ./...` 100.0%.

Native reviews (all approved and acknowledged):
- Lineage `review-c3571f327576f880` on `6417088..184873e`.
- Lineage `review-f04c5758be6fde76` on the whole branch.
- Lineage `review-5ba678268d6acce9` on the whole branch (third review).
- Lineage `review-7d591f8762aa97c2` on the whole branch (fourth review), approved and
  acknowledged.
Their advisory findings are addressed by T7 to T12.

Known follow-ups (not in this change):
- `Reserve` idempotency only holds while the driver still holds the route. A late
  duplicate `Reserve` with an old route id after release/complete would reserve the
  driver again, so the application layer must deduplicate by idempotency key
  (BASE_TECNICA 16.4) and never call `Reserve` for a finished route.
- Retries of `Release`/`CompleteRoute` after they succeeded return
  `ErrInvalidTransition` (the driver is no longer in the origin state); the
  application layer must treat them as already done.

## Route declaration

Trigger: writer (2+ non-trivial files across contracts, module, domain) plus
preparation (mapping already delegated and returned). One bounded writer,
sequential tasks, one commit per task.

## Acceptance criteria

- `go test -race ./...` passes in `services/driver-service` and still passes in
  `services/auth-service` and `services/api-gateway`.
- `gofmt -l` and `go vet ./...` clean.
- Proto follows auth.v1 conventions; generated code committed and consistent.

## Delivery

Forecast under ~400 authored changed lines excluding generated code. Strategy:
`ask-on-risk` (default). One PR slice expected.

## Progress / evidence

- Mapping of auth-service pattern: done (subagent), 2026-10-09.
- T1 commit 38a1e74 (buf lint clean, buf generate, auth generated files unchanged).
- T2 commit 2948816 (build ok; auth-service and api-gateway tests pass).
  testify require and go.sum added in T3 (go mod tidy drops unused deps).
- T3: RED = compile failure, undefined domain.Status etc; GREEN observed. -race unavailable (no cgo/gcc), plain go test used.
- T4: RED = compile failure (undefined domain.NewDriverInput, Driver, NewDriver); GREEN observed. Added assumptions: NewDriver constructor (originally started OFFLINE, changed to AVAILABLE in T6), active, version 1, ErrInvalidUserID, NormalizePhone.
- Commits: T1 38a1e74, T2 2948816, T3 143a827, T4 b93c3b4.
- Follow-up T5/T6 done: commits 5869f00, bf3ba15, cb5ad42; `-race` verified with
  gcc 15.2.0 (see T5/T6 evidence).
- T7 done: commits b094925, a5bd4dd, 509ae84, 1d92742 (see T7 evidence).
- T8 done: eaa033c. T9 done: 720a6d5.
- T10 done: a3470c1. T11 done: 6415cd7. T12 done: 954e60e.
- Native reviews: six approved and acknowledged (lineages review-c3571f327576f880,
  review-f04c5758be6fde76, review-5ba678268d6acce9, review-7d591f8762aa97c2,
  review-dac396f781005ba0 and review-5c783aee87e1d3c5 on the whole branch).
- Engram mirror `odd/driver-service-domain/tasks`: saved (backend-pikka).
- Decision 2026-10-09: `ReserveDriverResponse` keeps returning the full `Driver`
  (name and phone) on purpose: at delivery the driver must be able to contact the
  customer and the customer the driver. `FindAvailableDrivers` stays minimal
  (`DriverSummary`). Documented in `drivers.proto`.
- Decision 2026-10-09: the `FindAvailableDrivers` limit (default 50, maximum 100) is
  kept as an initial value. The user did not receive an explanation of it and asked
  to leave it; it is NOT a validated business number and can change before release.
- Decision 2026-10-09: delivery is chained PRs. The chain strategy
  (stacked-to-main or feature-branch-chain) is pending the user's choice.
- Decision 2026-10-09: the security scan is an active API scan; it is tracked in
  its own feature document and branch (`odd/tasks/security-scan.md`).

## Next step

Feature complete, reviewed and polished. Pending: the user's choice of chain
strategy, then slice the branch into chained PRs, push and open them (the user
confirms the push).
