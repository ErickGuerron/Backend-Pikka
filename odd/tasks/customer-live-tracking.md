# Feature: customer-live-tracking

Branch: `feat/tracking-customer-read`, base `develop`. Slices are stacked PRs (see Delivery).

## Objective

A CUSTOMER can follow one of their own orders in real time: the order status, and the
assigned driver's ETA (distance and duration) from the driver's latest GPS position. Updates
arrive over a WebSocket through the Gateway. This is the customer view of the tracking
pipeline built in the previous stack (#50 to #55).

## Scope

- In:
  - Order Service: customer-scoped single-order read (`GetMyOrder`) that also returns the
    order's `route_id`.
  - Driver Service: internal RPC `GetDriverByRouteID` (route -> reserved driver).
  - Gateway: `GET /api/v1/orders/{id}/tracking` (JSON snapshot) and
    `GET /api/v1/orders/{id}/tracking/ws` (WebSocket push), both role CUSTOMER.
  - Gateway: `EstimateArrival` client wrapper for Tracking Service.
- Out:
  - Stops before the customer's stop ("posicion de la parada", ADR 0008). Routing does not
    persist the stop sequence yet. Deferred to a follow-up feature.
  - Order status transitions PICKED_UP / IN_TRANSIT (Sprint 5, Anthony). The view shows
    whatever status the order has; new statuses will appear without contract changes.
  - Frontend views (Frontend-Pikka). Next feature after this one.
  - Public tracking link. ADR 0004 is superseded by ADR 0007; the customer has an account.
  - Driver name or phone (ADR 0008 forbids it for the customer).

## Constraints

- Authority: ADR 0007 (customer accounts, CUSTOMER role), ADR 0008 (scope and permissions).
  The customer sees only their own order, and never another customer's order. A foreign or
  missing order returns `404`, never `403`.
- Authorization order (ADR 0008 section 3): authenticate, then role, then scope in the query.
  The query filters by customer id; there is no fetch-then-filter.
- Strict TDD. RED then GREEN for every behavior task.
- Position values and coordinates are never logged (same rule as the location endpoint).
- Mirror existing layouts: order-service hexagonal use cases, driver-service ports, gateway
  handlers with `callMeta` and `translate` error mapping.
- Conventional Commits in Spanish, no AI attribution. No commit or push without the user asking.

## Design assumptions

1. **Scoped read is a new RPC, `GetMyOrder`.** The existing `GetOrder` has no scope rule
   and is not implemented in the gateway, so it is not reused. `GetMyOrder` mirrors
   `CancelForCustomer` (`WHERE id=$1 AND customer_id=$2`).
2. **`route_id` is added to the `Order` message** (new field, backward compatible). The gateway
   uses it internally to find the driver; the customer response DTO does not expose it.
3. **Route to driver lookup lives in Driver Service** (`GetDriverByRouteID`), because
   `drivers.route_id` is already the source of truth. Index the column in a migration.
4. **WebSocket library: `github.com/coder/websocket`** (maintained; gorilla/websocket is in
   maintenance mode). Echo serves the upgrade on the existing router.
5. **WebSocket auth without tokens in the URL.** Browsers cannot set `Authorization` on a
   WebSocket. The client sends the JWT as a subprotocol: `Sec-WebSocket-Protocol: pikka.v1, <jwt>`.
   The gateway validates the JWT with the same verifier, checks role CUSTOMER and order
   ownership, then echoes `pikka.v1`. Tokens never appear in URLs or logs.
6. **Push model.** The gateway polls the backend every 5 s per open socket and sends a message
   only when the snapshot changes. Simple, no pub/sub infra yet. The poll interval is a config value.
7. **ETA rules.** No ETA when the order has no `route_id` or no driver reserved. When Tracking
   answers `NOT_FOUND` or `FAILED_PRECONDITION` (no position, or stale position), the snapshot
   says `eta: null` with `eta_unavailable_reason: "no_position" | "stale_position"`. Never a
   fabricated estimate. `INTERNAL` from Tracking returns the snapshot without ETA and an
   `eta_unavailable_reason: "unavailable"`, so a routing outage does not break the order view.
8. **Snapshot shape (customer-safe):** `order_id`, `status`, `delivery_address`,
   `delivery_window {start,end}`, `eta {distance_meters, duration_seconds}` or null,
   `eta_unavailable_reason`, `updated_at`. No driver identity, no other orders, no route.

## Tasks

- [x] S1 (PR 1, order-service): `GetMyOrder` RPC + `route_id` on `Order`, repo method
      `GetForCustomer`, use case, gRPC handler, migration if needed (none expected, the column
      exists). TDD: use-case tests (own order, foreign order -> not found, non-CUSTOMER -> denied).
      Check: `go test -race ./...` in services/order-service; `buf lint`.
  - Evidence (uncommitted, working tree on `feat/tracking-customer-read`):
    - RED: `go test -race ./internal/application/order/` -> build failed:
      `undefined: GetMyOrder`, `undefined: NewGetMyOrder`, `undefined: GetCommand`.
    - GREEN application: `go test -race -count=1 -v -run GetMyOrder ./internal/application/order/`
      -> 4/4 PASS (own order with route_id, foreign -> ErrOrderNotFound, non-CUSTOMER ->
      ErrNotCustomer with no repo call, unknown id -> ErrOrderNotFound).
    - GREEN gRPC: `go test -race -count=1 -v -run GetMyOrder ./internal/adapters/grpc/` -> 6/6 PASS.
      Written after the handler (not RED-first); the RED above covers the behavior.
    - Full suite: `go test -race -count=1 ./...` in services/order-service -> all packages ok.
    - `go vet ./...` and `go vet -tags integration ./internal/adapters/persistence/` -> exit 0.
    - `gofmt -l .` in services/order-service -> empty.
    - `buf lint` in contracts -> exit 0. `buf generate` (buf 1.50.0, protoc-gen-go v1.36.5,
      protoc-gen-go-grpc 1.5.1) touched only `gen/go/orders/v1/*`. `orders.pb.go` diff is large
      (673 lines) because the embedded raw descriptor is regenerated; generated, not authored.
    - `go build ./...` in contracts -> exit 0.
    - Persistence: `GetForCustomer` covered by `order_get_integration_test.go` (build tag
      `integration`, compiles). NOT executed: the test `setup` runs `TRUNCATE` on orders,
      customers and products, and the only Postgres running is the shared compose dev DB.
      Run `make test-integration` against a disposable database.
    - Design choices beyond the doc: `GetCommand` type (mirrors `CancelCommand`, since `Command`
      has no `OrderID`); `route_id` = 15 in `Order`; `GetMyOrderRequest{order_id=1}`,
      `GetMyOrderResponse{order=1}`; handler wired in `cmd/api/main.go`.
    - Not yet done: the gateway must not expose `route_id` in customer DTOs (S3/S4). Any RPC
      returning `Order` now carries it.
- [ ] S2 (PR 2, driver-service): `GetDriverByRouteID` RPC, port `FindByRouteID`, postgres adapter,
      index migration on `drivers.route_id`. TDD: found, not found, blank route_id -> INVALID_ARGUMENT.
      Check: `go test -race ./...` in services/driver-service.
- [ ] S3 (PR 3, gateway): `grpcclient/tracking.go` `EstimateArrival`; `grpcclient/orders.go`
      `GetMyOrder`; `grpcclient/drivers.go` `GetDriverByRouteID`; `CustomerOnly` middleware.
      TDD for the wrappers' error mapping. Check: `go test -race ./...` in services/api-gateway.
- [ ] S4 (PR 4, gateway): snapshot use case, `GET /orders/:id/tracking` and the WebSocket
      endpoint with subprotocol auth, poll loop, change detection, ETA rules of assumption 7.
      TDD: 404 for foreign order, 403 for DRIVER role, ETA null cases, ws auth rejects a bad
      token, no coordinates in logs. Check: gateway tests with `-race`; manual ws check with
      the compose stack (user verifies).
- [x] S5 (PR 5): OpenAPI for both endpoints, `docs` update. Check: OpenAPI lint 0 errors.
  Evidence in "S5 OpenAPI" below.

## Acceptance criteria

- A CUSTOMER gets their own order snapshot with status and, when a fresh position exists, the ETA.
- The same customer asking for another customer's order gets `404`, identical to a missing order.
- A DRIVER or OPERATOR token gets `403` on both endpoints.
- A WebSocket client receives a snapshot on connect and a new message whenever the status or ETA changes.
- Without a fresh position the snapshot shows `eta: null` with a reason; it never shows an old ETA.
- Driver identity and coordinates do not appear in responses or logs for this feature.

## Delivery

- Base chain: `develop` -> PR 1 (order) -> PR 2 (driver) -> PR 3 (gateway clients) -> PR 4
  (gateway snapshot + ws) -> PR 5 (OpenAPI). PR 1 and PR 2 are independent of each other.
- Size: each slice measured with `git diff --shortstat` against its base after commit. Budget 400.

## Progress / evidence

- Research (read-only, explore mapper): confirmed the gaps listed in the Tasks section.
  - `GetOrder` RPC exists but is unimplemented in the gateway; no scope rule.
  - `route_id` is stored in `orders` (migration 000011) and not exposed in the proto.
  - No route to driver RPC; `drivers.route_id` exists and is not indexed.
  - Ordered stops are not persisted or exposed anywhere (deferred).
  - `GetDriverPosition` does not exist; `EstimateArrival` does.
  - No WebSocket library in the gateway; no CUSTOMER-only middleware.
- Correction: the previous tracking doc referenced ADR 0004 (public link). ADR 0004 is superseded
  by ADR 0007. This feature follows ADR 0007 and ADR 0008.

## P1 stop sequence and status transitions

Scope: order-service only (routing-service and gateway are later slices). Uncommitted on
`feat/tracking-stop-status`. Not committed, not pushed.

- Contract (no RED possible, schema only): `OrderStop{order_id, sequence}`, `stops = 3` on
  `AssignOrdersToRouteRequest` (`order_ids = 2` kept), `AdvanceRouteStop` RPC with
  `AdvanceRouteStopRequest{route_id, order_id, target_status}`. Migration
  `000013_order_stop_sequence`: `orders.stop_sequence INTEGER NULL CHECK (>= 1)` plus the partial
  unique index `(route_id, stop_sequence) WHERE stop_sequence IS NOT NULL`.
  - `buf lint`: exit 0. `buf generate` with buf 1.50.0, protoc-gen-go v1.36.5, protoc-gen-go-grpc
    1.5.1 (headers unchanged). `go build ./...` in contracts: exit 0.
- RED (TDD, before any implementation):
  - `go test ./internal/domain/` -> build failed: `too many arguments in call to order.AssignToRoute`,
    `undefined: ErrInvalidStopSequence`, `StopSequence undefined`.
  - `go test ./internal/application/order/` -> build failed: `unknown field Stops`, `undefined: RouteStop`,
    `undefined: AdvanceRouteStop`.
  - `go test ./internal/adapters/grpc/` -> build failed: `undefined: orderapp.NewAdvanceRouteStop`,
    `unknown field AdvanceStop`, `undefined: orderapp.RoleDriver`.
  - `go vet -tags integration ./internal/adapters/persistence/` -> `not enough arguments in call to
    o.AssignToRoute` (existing test, updated to the new signature with 0).
- GREEN, with two failures caught by the new tests during the cycle:
  - `TestAdvanceRouteStopTransitionTable`: a rejected step returned the zero `Order` (from
    `TransitionTo`). Fixed: `AdvanceRouteStop` returns the untouched order on error.
  - `TestAdvanceRouteStopErrorCodes`: a test bug (the seeded order id did not match the request).
    Fixed in the test.
- Final checks (in `services/order-service`):
  - `go test -race -count=1 ./...`: all packages ok (exit 0).
  - `go vet ./...`: exit 0. `go vet -tags integration ./internal/adapters/persistence/`: exit 0.
  - `gofmt -l .`: empty. 26 PASS among the new tests (domain, use case, gRPC), 0 FAIL.
  - Gateway, routing-service and driver-service `go build ./...`: exit 0 (the proto change is additive).
- Integration tests (`order_stop_sequence_integration_test.go`, `order_route_assignment_integration_test.go`):
  compile only. NOT executed, same reason as S1 (`setup` truncates the shared DB). Run them against a
  disposable database.
- Design choices beyond the spec:
  - Legacy `order_ids` assigns sequence 0 (unassigned, stored as NULL). Stops are not invented from order.
  - Stop sequence checks: `stops` entries need sequence >= 1 and unique (use case), and the database index
    also enforces it. A 23505 on that index maps to `ErrDuplicateStopSequence` (INVALID_ARGUMENT).
  - `AdvanceRouteStop` requires role DRIVER (defense in depth; the Gateway also checks ownership). Other
    roles get PERMISSION_DENIED.
  - `ErrInvalidTransition` is now mapped to FAILED_PRECONDITION in `mapOrderError` (it was INTERNAL). Other
    callers of that error are not reachable from the gRPC handlers in the current code paths.
  - The optimistic check the feature asked for does not exist in `Review`. `Review` (and the new method) locks
    the row with `SELECT ... FOR UPDATE` and increments `version`; it never compares an expected version.
    So there is no stale-version test. A real version compare would be a new pattern for the team.
  - `stop_sequence` is not exposed in the `Order` message (not in the spec). Routing and the customer view
    need it in a later slice.
- Size: 981 authored lines (342 production incl. proto and migration, 639 tests), excluding `*.pb.go`.
  This is over the 400 budget. Per PROTOCOL, that is a stop point: the split is not decided yet.
- Still open:
  - routing-service must send `stops` with the sequence it computes (it still sends `order_ids`).
  - driver-service route lookup (S2) and gateway wiring for `AdvanceRouteStop` (route ownership check).
  - Cancelling a `ROUTE_ASSIGNED` order keeps its `stop_sequence` (not cleared). Routing must ignore cancelled orders.
  - The advance does not write a customer notification (no spec for it).
  - Run the integration tests against a disposable database.

## P2 routing sends stops

Scope: routing-service only (order-service read-only). Uncommitted on `feat/tracking-stop-status`.
Not committed, not pushed.

- Change: the `OrderGateway.AssignToRoute` port takes `[]domain.RouteStop` (planned sequence) instead of
  `[]string` order IDs. The gRPC adapter sends `stops` (`OrderStop{order_id, sequence}`) and no longer sends
  `order_ids`. `plan.Generate` passes `route.Stops`. The domain (planner, route) is unchanged.
- RED (TDD, test written first): `go test -count=1 ./internal/adapters/clients/` -> build failed:
  `cannot use stops (variable of type []domain.RouteStop) as []string value in argument to gw.AssignToRoute`.
  The test asserts the stops reach Order Service with their sequence and that `order_ids` is empty.
- GREEN: same package -> PASS. `generate_test.go` now also asserts sequences are 1..n per route.
- Checks (in `services/routing-service`):
  - `go test -race -count=1 ./...`: all packages ok (exit 0).
  - `go vet ./...`: exit 0. `gofmt -l .`: empty.
  - `go build ./...` in `contracts`: exit 0. `go build ./...` in `services/order-service`: exit 0 (P1 compiles).
- Not done: no end-to-end run against a live order-service (the gRPC test uses a fake server). The
  order-service `AssignOrdersToRoute` path with `stops` is covered by P1 tests, not re-run here.

## P3 driver lookup by route

Scope: driver-service (S2 of the Tasks list), contract and persistence included. Uncommitted on
`feat/tracking-stop-status`. Not committed, not pushed. P1 and P2 files untouched.

- Contract: `rpc GetDriverByRouteID(GetDriverByRouteIDRequest) returns (GetDriverByRouteIDResponse)`,
  request `route_id = 1`, response `driver_id = 1`, `user_id = 2` (ids only, no name or phone).
  Codes documented in the proto comment: INVALID_ARGUMENT (blank route_id), NOT_FOUND (no driver holds the route).
- Migration `000004_index_drivers_route_id`: `CREATE INDEX drivers_route_id_idx ON drivers (route_id) WHERE route_id <> ''`
  (partial; down drops it).
- Port `DriverRepository.FindByRouteID`; implemented in the postgres adapter and the in-memory adapter.
- Use case `Service.FindByRouteID`: trims, blank -> `domain.ErrInvalidRouteID`, otherwise repository lookup.
- gRPC `DriverServer.GetDriverByRouteID`: blank -> INVALID_ARGUMENT, `ErrDriverNotFound` -> NOT_FOUND through `toStatus`.
  `cmd/api/main.go` not changed: the handler is served by the existing `NewDriverServer` registration.
- RED (before any production code): `go test -count=1 ./internal/application/driver/ ./internal/adapters/grpc/` ->
  build failed: `svc.FindByRouteID undefined` (4 call sites), `client.GetDriverByRouteID undefined`,
  `undefined: driversv1.GetDriverByRouteIDRequest`.
- GREEN: `go test -count=1 -v -run ByRouteID` in both packages -> 7/7 PASS (use case: found, not found,
  other route not returned, blank `""` and `"   "` -> ErrInvalidRouteID; gRPC: only ids returned, NOT_FOUND,
  blank -> INVALID_ARGUMENT).
- Design choice: the postgres and memory adapters also refuse an empty route id. `route_id = ''` would match every
  free driver, since free drivers store an empty route.
- Checks (in `services/driver-service`):
  - `go test -race -count=1 ./...`: all packages ok (exit 0).
  - `go vet ./...`: exit 0. `go vet -tags integration ./...`: exit 0 (compile only).
  - `gofmt -l .`: empty.
- Checks (contracts): `buf lint`: exit 0. `buf generate` (buf 1.50.0, protoc-gen-go v1.36.5, protoc-gen-go-grpc 1.5.1,
  headers unchanged) regenerated `gen/go/drivers/v1/*` and also rewrote `gen/go/orders/v1/*` with the same tools
  (no orders proto change in this slice). `go build ./...` in contracts: exit 0.
- `go build ./...` in `services/api-gateway`: exit 0.
- Integration: `driver_repository_route_integration_test.go` (build tag `integration`) compiles. NOT executed:
  `openRepo` truncates `drivers`, and the only Postgres running is the shared compose dev DB. Migration 000004 is
  not applied to any database yet. Run it with `DRIVER_TEST_DATABASE_URL` against a disposable DB.
- Not done: the gateway client `GetDriverByRouteID` (S3). The ETA snapshot (S4).

## Next step

S3 (gateway `grpcclient/drivers.go` wrapper for `GetDriverByRouteID`, plus the other S3 wrappers), then S4, S5.
Before S4 closes, run the integration tests against a disposable DB for S1 and P3 (`DRIVER_TEST_DATABASE_URL`).

## P4 gateway (S3 + S4, plus ListRouteStops)

Scope: api-gateway, the `ListRouteStops` RPC in order-service (the only order-service change allowed
in this slice), and the proto/gen for it. Uncommitted on `feat/tracking-stop-status`. Not committed,
not pushed. P1-P3 files untouched.

- Contract: `rpc ListRouteStops` on OrderService, `ListRouteStopsRequest{route_id}`,
  `ListRouteStopsResponse{stops}`, `RouteStopSummary{order_id, sequence, status}`. `buf lint` exit 0.
  `buf generate` (buf 1.50.0) regenerated `gen/go/orders/v1/*`.
- order-service: `ports.RouteStopRepository`, `orderapp.ListRouteStops` (blank route -> ErrInvalidRouteID),
  persistence `ListStopsByRoute` (`route_id = $1 AND stop_sequence IS NOT NULL ORDER BY stop_sequence`),
  gRPC handler, wiring in `cmd/api/main.go`.
- Gateway: `CustomerOnly` middleware; `grpcclient` wrappers (`Orders.GetMyOrder/ListRouteStops/AdvanceRouteStop`,
  `Drivers.GetDriverByRouteID`, `Tracking.EstimateArrival`); snapshot `GET /orders/:id/tracking`;
  WebSocket `GET /orders/:id/tracking/ws` (`github.com/coder/websocket` v1.8.15, subprotocol auth);
  driver stop `PATCH /drivers/me/routes/:routeId/stops/:stopId/status`; config `TRACKING_POLL_INTERVAL` (default 5s).

### RED (strict TDD)

- order-service, before any production code: `go test ./internal/application/order/ ./internal/adapters/grpc/`
  -> build failed: `undefined: NewListRouteStops`, `ports.RouteStopRepository undefined`,
  `unknown field RouteStops`, `ListRouteStops undefined`, `ordersv1.ListRouteStopsRequest undefined`.
- Gateway, compile RED: `go test ./...` -> build failed: `cfg.TrackingPollInterval undefined`,
  `ow.GetMyOrder/ListRouteStops/AdvanceRouteStop undefined`, `dw.GetDriverByRouteID undefined`,
  `tw.EstimateArrival undefined`, `opts.TrackingTicker undefined`.
- Gateway, behavior RED (after plumbing: fields, wrappers, interfaces; no routes, no handlers):
  driver stop 6 tests failed (501 instead of 200/400/404/422); snapshot 10 tests failed (403/404 where 200
  was expected, including 7 StopsBefore cases); WebSocket 5 tests failed (401 on upgrade, 404 not reached).
  The 403 tests for non-customer roles and the logs guard passed at this stage because the routes did not
  exist; they are regression guards, not new behavior.
- Config and wrapper tests: their code was written in the plumbing step, before the behavior RED run. They
  were red at compile time only.

### GREEN

- order-service: `go test -race -count=1 -v -run ListRouteStops` in application/order and grpc -> 7/7 PASS.
  Full package suite `go test -race -count=1 ./...` -> all packages ok.
- Gateway: `go test -race -count=1 ./...` -> all packages ok (grpcclient, http, config, jwt).
- Mutation check: removing the WebSocket skip in `ContextTimeout` made
  `TestTrackingWS_SurvivesRequestTimeout` fail (`the handler did not take the poll tick`); restored, PASS.

### Checks (real output)

- `services/api-gateway`: `go test -race -count=1 ./...` ok; `go vet ./...` exit 0; `gofmt -l .` empty.
- `services/order-service`: `go test -race -count=1 ./...` ok; `go vet ./...` exit 0;
  `go vet -tags integration ./internal/adapters/persistence/` exit 0; `gofmt -l .` empty.
- `contracts`: `go build ./...` exit 0; `buf lint` exit 0.
- `services/routing-service` and `services/driver-service`: `go build ./...` exit 0 against the new contract.
- Size (authored lines, excluding `*.pb.go` and `go.sum`):
  - P4 alone: about 1,870 added lines (gateway ~1,547, order-service ~300, contract ~23). Over the 400 budget.
  - Working tree vs HEAD (P1-P4 together, uncommitted): `git diff --shortstat` = 45 files, 1,556 insertions,
    239 deletions (includes generated code); tracked authored +766/-97 plus 2,525 lines in 26 untracked files
    (both excluding `*.pb.go`).

### Not verified

- Live WebSocket against the running compose stack (needs Docker). Not run.
- Real browser handshake from the frontend (Origin check tested only with no Origin header).
- Socket lifetime beyond the server `WriteTimeout` (15 s in `main.go`) on a real listener. Read/write deadlines
  are cleared before the upgrade, but no test exercises a live server past 15 s.
- The production poll clock (`defaultTicker`, `time.NewTicker`). Tests inject the clock.
- Integration tests for `CountStopsBefore` (`order_route_stops_integration_test.go`) compile only (`-tags integration`).
  Not executed: `setup` truncates the shared dev DB. Run against a disposable database.

### Decisions beyond the spec (review these)

1. `stops_before` also excludes `CANCELLED`. The spec only excludes DELIVERED and NOT_DELIVERED. A cancelled
   order keeps its `stop_sequence` (P1 gap), so counting it would give the customer a wrong number.
   Revert = remove one case in `isOpenStop`.
2. `ListRouteStops` had no role gate (any authenticated identity could list a route's order ids and statuses).
   Resolved by the security fix below: the RPC was replaced by `StopsBeforeForOrder`.
3. `updated_at` is the server time of the snapshot build (Order has no updated timestamp). Change detection
   ignores it, so a poll only pushes when status or ETA changed.
4. WebSocket rejections happen before the upgrade (HTTP 401/403/404), not as close frames. The socket is
   only opened for a CUSTOMER that owns the order.
5. `ContextTimeout` skips WebSocket upgrades. Without it the socket dies at `REQUEST_TIMEOUT` (10 s by default).
6. Driver stop: 404 for both foreign and unknown routes (same message). The route is only registered when
   `Orders` is wired; otherwise the old 501 stays.
7. Customer tracking routes register only when Orders, Drivers and Tracking are all wired.
8. A failed poll is skipped silently and retried on the next tick. Degraded parts log a warning with the gRPC
   code only (no ids, coordinates or tokens).
9. `TrackingTicker` is an exported Options field, used as the test seam for the poll clock.

### Next step

- Split the slice before any PR: P4 is about 4.7x the 400-line budget. Suggested cut: (a) order-service
  `ListRouteStops` + driver stop endpoint + config; (b) customer snapshot + WebSocket. Decision pending with the user.
- Live check with compose (WebSocket lifetime, real Origin, poll). Then S5 (OpenAPI).

## Security fix: scoped stops_before

Finding: `ListRouteStops(route_id)` returned the order id, sequence and status of every order on a route to
any authenticated identity that knew the route id. A customer could see other customers' orders.

Fix: `ListRouteStops`, `ListRouteStopsRequest`, `ListRouteStopsResponse` and `RouteStopSummary` are removed.
Replaced by `rpc StopsBeforeForOrder(StopsBeforeForOrderRequest{order_id}) returns (StopsBeforeForOrderResponse{stops_before})`.

- Contract: `buf generate` (buf 1.50.0), `buf lint` exit 0. The drivers gen files did not change.
- order-service: `ports.RouteStopRepository.CountStopsBefore(orderID, customerID)`. One scoped query: the
  order row is filtered by `customer_id`, then it counts same-route orders with a smaller `stop_sequence` and a
  status other than DELIVERED, NOT_DELIVERED or CANCELLED. Foreign or unknown order -> NOT_FOUND. The use case
  `StopsBeforeForOrder` checks the CUSTOMER role before any repository call (non-CUSTOMER -> PERMISSION_DENIED,
  as `GetMyOrder`). An order without route or sequence -> 0.
- Gateway: `Orders.StopsBeforeForOrder` with the customer's own order id. The client-side counting and
  `isOpenStop` are removed. `stops_before` keeps its meaning: a number, null when the order has no route or the
  call fails.

### RED (before any production change)

- order-service `go test ./internal/application/order/ ./internal/adapters/grpc/` -> build failed:
  `undefined: StopsBeforeForOrder`, `undefined: NewStopsBeforeForOrder`, `undefined: StopsBeforeCommand`,
  `unknown field StopsBefore`, `ordersv1.StopsBeforeForOrderRequest undefined`.
- `go vet -tags integration ./internal/adapters/persistence/` -> `orders.CountStopsBefore undefined`.

### GREEN

- order-service: `go test -race -count=1 ./...` ok (application/order and grpc include the new scope tests).
- Gateway mutation check: forcing `stops_before` to 0 made `TestTrackingSnapshot_StopsBeforeIsTheServiceCount`
  (open_stops_ahead) and `TestTrackingSnapshot_OwnOrderWithFreshPosition_ReturnsETAAndStopsAhead` fail. Restored.
- Gateway RED was not captured in its own run: its tests were written before the contract existed and first ran
  in the GREEN phase. The mutation check above is the evidence that they test the count.

### Checks (real output)

- `services/order-service`: `go test -race -count=1 ./...` exit 0; `go vet ./...` exit 0;
  `go vet -tags integration ./internal/adapters/persistence/` exit 0; `gofmt -l .` empty.
- `services/api-gateway`: `go test -race -count=1 ./...` exit 0; `go vet ./...` exit 0; `gofmt -l .` empty.
- `contracts`: `go build ./...` exit 0; `buf lint` exit 0.
- `services/routing-service` and `services/driver-service`: `go build ./...` exit 0.

### Not verified

- Integration tests `TestCountStopsBeforeCuentaParadasAbiertasAntesDeLaMia` and
  `TestCountStopsBeforeDeOtroClienteNoExiste` compile only. They were not run, because `setup` truncates the
  shared dev DB. Run them against a disposable database.
- The count still tells a customer how many open stops are ahead on the route. That is the agreed contract
  (a number only, no ids or statuses).

## S5 OpenAPI

Scope: `contracts/openapi/openapi.yaml` only (plus this file). Uncommitted on `feat/tracking-stop-status`.
Not committed, not pushed.

- Added `GET /api/v1/orders/{id}/tracking` (CUSTOMER) and `GET /api/v1/orders/{id}/tracking/ws` (WebSocket,
  documented as GET with `x-websocket: true`; the lint accepts the extension). Schema `TrackingSnapshot`
  (snake_case, as the Gateway emits it) and `StopStatusResponse`.
- `PATCH /api/v1/drivers/me/routes/{routeId}/stops/{stopId}/status` changed from 501 to the implemented
  behavior: 200, 400, 401, 403, 404, 415, 422, 503. The 501 was removed because `main.go` always wires Orders,
  so the route is live. The other driver stubs (`GET /drivers/me/route`, `POST .../result`) are unchanged.
- Codes beyond the requested list come from the code paths: 503 on the snapshot (gRPC Unavailable), 415 for
  non-JSON bodies (`bindJSON`), 422 for an illegal step (`FAILED_PRECONDITION` maps to 422 in `grpcToHTTP`).
- Drivers tag description updated to match the new status of the stop route.
- Descriptions of the new blocks are in English, as the artifact rule requires. The rest of the file is Spanish.

Lint (repo command: `npx --yes @redocly/cli@1.34.0 lint --config contracts/openapi/redocly.yaml contracts/openapi/openapi.yaml`):
- Before: `valid`, 0 errors, 3 warnings (`operation-2xx-response` on GET /drivers/me/route, PATCH stop status, POST result).
- After: `Woohoo! Your API description is valid`, 0 errors, 3 warnings: GET /drivers/me/route, POST result, and
  the WS GET (its only success response is `101`, which the rule does not count as 2XX). The PATCH warning is gone.
- YAML parse (`python -c "import yaml..."`): ok, 34 paths.
- `go build ./...` in `services/api-gateway`: exit 0 (embed compiles). `go vet ./internal/adapters/http/`: exit 0.
  `go test -run "Docs|OpenAPI"` in the same package: ok.

Not verified:
- The spec is hand-written. No automated test compares it with the Go structs or handlers; field names and codes
  were checked by reading the code (`tracking_snapshot.go`, `tracking_ws.go`, `driver_stop_handlers.go`, `errors.go`).
- 415, 422 and 503 for the stop route, and 503 for the snapshot, come from code paths read, not from a running server.
- Live WebSocket against the compose stack, and the 1008 close on a client data message (from the library source,
  `CloseRead`, v1.8.15; not exercised by a test here).
- The gateway's `/openapi.yaml` and Scalar `/docs` were not loaded in a running server or browser.
- The 101 warning stays because `101` is the real response. Removing it would mean documenting a 200 that does not exist.
