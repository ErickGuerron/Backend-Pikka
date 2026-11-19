# Feature: routing-plan-contract (phase 1)

Branch: `feat/routing-plan-contract` on `develop`. Route: delegated direct only for reads; the parent writes inline. Delivery strategy: single PR, about 400 authored lines (proto and generated code excluded). Test runner: `go test ./...` per module, `make proto` for contract generation.

## Specs

S1. POST /api/v1/routes/plan returns, per route, `driverName` (the name of the reserved driver) and `stops` (each with `orderId`, `address`, `sequence`), ordered by `sequence` starting at 1.
S2. The same response includes `planId` and `unassignedOrderIds`. Phase 1: `planId` is derived as `plan-<deliveryDate>` and is NOT persisted; `unassignedOrderIds` is always an empty array. These are documented as phase-1 placeholders, not real plan state.
S3. The failure policy does not change: FAILED_PRECONDITION still stops the run, and no route is partially reverted.
S4. The contract change is additive only: new fields and a new message, no renumbering. `buf breaking` must pass against the base.
S5. Gateway JSON keeps the existing fields `routeId`, `driverId`, `orderCount`; new fields are added beside them. OpenAPI documents the new fields.
S6. Out of scope: persisted plans, partial planning with real unassigned orders, insert or move of stops.

## Tasks

T1 (S4) proto: `PlannedStop`, `PlannedRoute.driver_name = 4`, `PlannedRoute.stops = 5`, `GeneratePlanResponse.plan_id = 2`, `GeneratePlanResponse.unassigned_order_ids = 3`; regenerated with `make proto`. Route: inline. Status: done.
T2 (S1) routing-service: `PlannableOrder.Address` and `RouteStop.Address`; `DriverPool.Reserve` returns the driver name; `AssignedRoute.DriverName`. Route: inline. Status: done. Evidence: `go test ./...` green.
T3 (S1, S2) routing gRPC server: maps stops, names, planId (`plan-<date>`) and an empty unassigned list. Guard added for a result without planned stops. New test `TestGeneratePlanMapsNamesStopsAndPlanID`. Route: inline. Status: done.
T4 (S1, S2, S5) gateway: `generatePlanResponse` gains planId, unassignedOrderIds; routes gain driverName and stops; `routeId`, `driverId`, `orderCount` kept. `routing_handlers_test.go` updated. Route: inline. Status: done. Evidence: `go test ./...` green for api-gateway.
T5 (S5) OpenAPI: `PlannedStop` schema, new fields documented. Route: inline. Status: done. Evidence: YAML parses; redocly not installed locally.
T6 verify: routing-service and api-gateway tests green; `buf lint` exit 0; `buf breaking --against .git#ref=HEAD,subdir=contracts` exit 0. Status: done.

## Log

L1 (user, verbatim): "ya ahora si explra el frontend para ver que cambios ubo y ve donde nos quedamos en bakcend para ver si ya podemos  aplicarlo en front"
L2 (user, verbatim): "haslo justo en ese orden" (correct the web routing paths first, then mobile cancel and stop status, then customer tracking, then docs, then loose files).
L3 (user answer, verbatim option): "Ampliar backend (Recomendado)" for the routing contract mismatch.
L4 (user answer, verbatim option): "Fase 1 ahora (Recomendado)" for the scope: additive fields now, persisted plan and unassigned orders later.
L5 Finding: the web RoutesPage needs `driverName`, `stops`, `plan.id` and `unassignedOrderIds`; the gateway returned only `routeId`, `driverId`, `orderCount`. Explorer evidence: stops are already computed in `domain.Route` and dropped in `routing_server.go:53-60`; driver name is returned by `ReserveDriver` (`drivers.proto:165-171`) and dropped by `DriverPool.Reserve`; order address is in `orders.proto:84-88` and dropped by `clients.go:53-64`.
