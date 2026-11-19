# Feature: order-route-assignment

Branch: `feat/order-route-assignment` (from `origin/develop` @ 162b0eb). Not on the routing branches.

## Objective

Let the Order Service expose pending orders for route planning and atomically
assign a set of them to a route. This is slice 2 of the Routing Service sprint
(Erick).

## Scope

- In: `zone_id` on `Order` in `orders.v1` (field 11); new RPC
  `AssignOrdersToRoute` (PENDING to ROUTE_ASSIGNED, all-or-nothing); implementation
  of the already-declared `GetPendingOrders` (PENDING only, by `delivery_date`).
- Out: `routing.proto`, gateway and OpenAPI route-plans, Idempotency-Key, cancel
  notification, concurrency tests, k6.

## Verified starting state (2026-10-15)

Already on `develop`, not to be re-done:
- `Order.ZoneID` in the domain; `zone_id` column (migration 000004) and persistence.
- `StatusPending -> StatusRouteAssigned` allowed in `domain/status.go`;
  `Order.TransitionTo` exists.
- `GetPendingOrders` declared in the contract, returns `Unimplemented` in the server.

Missing before this change:
- `zone_id` not exposed in the proto `Order` (done in T1).
- `AssignOrdersToRoute` did not exist (done in T1).
- No repository, use case, or gRPC handler for pending orders or route assignment.

## Constraints

- Contract first: T1 merges before T2 and T3.
- `buf breaking` must pass against `develop`.
- TDD: strict. Runner: `go test -race ./...` per module (`make test`).
- Commits: Conventional Commits, no AI attribution. No push or PR without the user.
- Capacity per route comes from the zone (Zone Service `max_orders_per_route`),
  passed by the caller. Routing does not call ClassifyPoint.

## Design assumptions

- `AssignOrdersToRoute` is all-or-nothing: if any order is not PENDING (or does not
  exist), nothing changes. Not-PENDING returns ABORTED naming the first offending id;
  unknown returns NOT_FOUND.
- `GetPendingOrders` returns only PENDING orders, never ROUTE_ASSIGNED ones, ordered
  by `created_at` ascending, then `id`.
- `AssignOrdersToRoute` does not store `route_id` on the order yet (no column in
  scope). The route link is a follow-up with the Routing persistence.

## Tasks

- [x] T1 Contract: `zone_id = 11` on `Order`; `rpc AssignOrdersToRoute` with
      `AssignOrdersToRouteRequest {route_id, order_ids}` and
      `AssignOrdersToRouteResponse {orders}`. Route: inline (contract only, no RED).
      Evidence: see Progress.
- [x] T2 Domain (TDD): `Order.AssignToRoute()` allowed only from PENDING, returns
      `ErrOrderNotPending` and leaves the order unchanged otherwise. Route: inline
      (delegated writer failed, see Progress). Check: `go test -race ./...` in domain: ok.
- [x] T3 Application, ports, persistence, gRPC (TDD): `RouteAssignment` use cases
      (`PendingByDate`, `Assign`), port `RouteAssignmentRepository` (new file, the
      existing repository fakes are untouched), `ListPendingByDate` and
      `AssignToRoute` in one transaction with `FOR UPDATE` ordered by id,
      handlers and error mapping, `orderToProto` sets `ZoneId`, wiring in `main.go`.
      Route: inline. Check: see Progress.
- [ ] T4 Size check: 636 authored lines (excluding `contracts/gen` and `odd/`). Over
      the 400 budget. Split into two stacked PRs (see Delivery). Not yet applied.

## Acceptance criteria

- `go test -race ./...` passes in `services/order-service`.
- `buf lint` and `buf breaking` pass against `develop`.
- `AssignOrdersToRoute` is atomic: a test proves a partial failure leaves every order
  unchanged.
- `GetPendingOrders` never returns ROUTE_ASSIGNED orders.

## Delivery

Base `develop`. Stacked PRs into `develop` as chosen by the user for the routing
sprint. Measured at T4 with `git diff --numstat` (authored lines only):

- PR A `feat/order-route-assignment-domain` -> `develop` (~264 lines): proto
  (20), domain (56), ports (21), application use cases and tests (167).
- PR B `feat/order-route-assignment-persistence` -> PR A (~378 lines): persistence
  and integration test (220), gRPC handlers, mapping and tests (144), `main.go` (14).

Both under 400. Not committed: the user decides commits, push and PRs.

## Progress / evidence

- 2026-10-15: resume point from Engram (`sdd/routing-service-domain/resume-point`).
  The earlier `sdd-propose` run was cancelled and is not reused.
- T1 done: `orders.proto` +20 lines. `buf lint` clean. `buf breaking` against
  `origin/develop` exit 0. `buf generate` changed only `contracts/gen/go/orders/v1`.
  `go build ./...` in `contracts` and `services/order-service` exit 0.
  `gofmt -l` clean.
- Build of `services/routing-service` fails on this branch because that module is
  only on the routing branches (expected, not part of this change).
- Delegation of T2+T3 to the `general` agent failed three times with
  `gpt-5.4-mini is not supported when using Codex with a ChatGPT account`. The
  work was done inline instead. No review or code was produced by the failed runs.
- T2+T3 TDD evidence (RED then GREEN):
  - Domain: RED = `undefined: AssignToRoute, ErrOrderNotPending`. GREEN with
    `go test -race` ok.
  - Application: RED = `undefined: NewRouteAssignment, AssignToRouteCommand`.
    GREEN with `go test -race` ok for the order package.
  - Integration (persistence): RED = `AssignToRoute undefined`. GREEN against
    PostgreSQL 16 (ephemeral container): `TestPendientesSoloPendingDeLaFecha`,
    `TestAsignarRutaTodoONinguno`, `TestAsignarRutaIDDesconocido`,
    `TestAsignarRutaOK`, and existing cancel tests pass with `-race`.
  - gRPC: RED = `unknown field RouteAssignment`. GREEN with `go test -race` ok.
- Full order-service run: `go vet ./...` and `go vet -tags integration ./...` clean;
  `gofmt -l .` clean; unit `go test -race ./...` all ok; integration suite all ok
  except `TestOrderMigrations`.
- `TestOrderMigrations` fails identically on a clean `origin/develop` worktree
  (`schema "auth" does not exist`, and `orders_delivery_date_status_idx` count 0). It
  depends on the shared dev database with other services' schemas. Not caused by
  this change; no migration was touched.

## Decisions applied (user, 2026-10-15)

1. Role check: `GetPendingOrders` and `AssignOrdersToRoute` require the propagated
   role ADMIN or OPERATOR (same rule as order review, `isOperator`). Missing identity
   returns UNAUTHENTICATED; CUSTOMER and other roles return PERMISSION_DENIED.
   Consequence: Routing must call with an operator identity. A service-to-service
   identity is not defined yet; decide before Routing integration.
2. `route_id`: migration `000011_order_route_id` adds nullable `orders.route_id TEXT`
   (routing ids are `{zone_id}#{n}`, not UUIDs). The domain stores it in
   `AssignToRoute(routeID)`, and the assignment `UPDATE` persists it. Not exposed in
   the proto `Order` yet.
3. Commits: split into two commits, A (contract, domain, ports, application, ODD docs)
   and B (migration, persistence, gRPC, wiring), so each PR stays under budget.

## Next step

Push and open PRs only when the user asks. Stacked: A -> develop, B -> A.
