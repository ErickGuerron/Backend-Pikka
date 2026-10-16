# Feature: routing-plan-generation

Branch: `feat/routing-plan-generation`, stacked on `feat/routing-service-planner` (on `feat/routing-service-types`, on `develop`). The Order Service contract it uses is already in `develop` (PRs #38 and #39).

## Objective

Routing application use case that plans the PENDING orders of a delivery date by zone
and capacity, then assigns each route to Order Service. This is the core of the
`POST /api/v1/route-plans/generate` flow (Sprint 3).

## Scope

- In: `ports.OrderGateway` and `ports.ZoneCapacities`; `plan.Generate.Run(date)`;
  per-zone capacity; date-scoped route ids; stop before assignment on planning errors.
- Out: gRPC adapters to Order and Zone (Anthony, Sprint 3), `routing.proto`, gateway
  route, persistence of routes, idempotency, concurrency invariants, k6.

## Constraints

- Stdlib only in `services/routing-service` (no new dependencies).
- TDD strict. Runner: `go test -race ./...` in `services/routing-service`.
- English identifiers and comments. Conventional Commits. No push or PR without the user.

## Design decisions

- Capacity is per zone (`max_orders_per_route`). The planner takes one capacity per
  call, so each zone is planned on its own.
- All zones are planned before the first assignment. A planning error changes no order.
- Route ids sent to Order Service are `{deliveryDate}/{zone}#{n}`. Planner ids repeat
  every day, so the date keeps them unique.
- Assignment is one all-or-none call per route. Routes are not atomic together. A failure
  returns the routes already assigned (`Result.Assigned`), so the caller can report them.
- A blank zone is rejected before any Zone Service call.

## Tasks

- [x] T1 Ports: `OrderGateway`, `ZoneCapacities`. Route: inline (interfaces only).
- [x] T2 Use case (TDD): `plan.Generate.Run`. Route: inline (2 files, under the write
      threshold for this slice; delegation to the `general` agent was not retried because
      it failed with a model configuration error in the previous slice).
      Evidence: the first run exposed one real defect (zone lookup happened before the
      order was validated for an empty zone), fixed by validating the zone first.
      Note: TDD was not strictly RED-first for this slice. Tests and implementation were
      written before the first run; the failure found by the run is recorded here.
- [x] T3 gRPC adapters (TDD): `OrderGateway` and `ZoneCapacities` over real gRPC in-memory (bufconn). RED = undefined constructors; GREEN with -race. Identity: the caller's `x-user-id`/`x-user-role` is forwarded, because Order and Zone authorize by the user role. Zone `GetZone` does not accept the `SERVICE` role (only `ClassifyPoint` does), so Routing acts as the operator who started the request instead of adding a role to Zone Service.
- [x] T4 Lint: golangci-lint (CI matrix includes routing-service) flagged `gocritic appendAssign` in `planner.go` from the planner branch; fixed with a pointer to the route. Lint 0 issues.
- [ ] T5 Size check and PR plan.

## Acceptance criteria

- `go test -race ./...` passes in `services/routing-service`.
- `go vet ./...` and `gofmt -l` clean.
- A failed planning or assignment never leaves an order half-assigned by this use case.

## Progress / evidence

- Tests: 7 scenarios in `internal/application/plan/generate_test.go`, all passing with
  `-race`. Domain tests still pass.

## Next step

Measure the diff (T3). Then the gRPC adapters to Order and Zone, which need the merged
`orders.v1` contract.
