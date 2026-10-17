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
- [ ] S5 (PR 5): OpenAPI for both endpoints, `docs` update. Check: OpenAPI lint 0 errors.

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

## Next step

S2 (driver-service `GetDriverByRouteID`), then S3, S4, S5. Before S4 closes, run the integration tests against a disposable DB for S1.
