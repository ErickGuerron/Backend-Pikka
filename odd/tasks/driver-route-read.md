# Feature: driver-route-read

Branch: `feat/driver-route-read` (from `feat/routing-plan-contract`). Route: parent writes inline per task; no writer delegation (small units). Delivery: single PR, about 400 authored lines (generated code excluded). Test runner: `go test -race -count=1 ./...` per Go module; `npx jest` in `Frontend-Pikka/apps/mobile` (blocked until the environment is fixed, see Progress); `npm run typecheck` in the mobile app.

## Specs

S1. `GET /api/v1/drivers/me/route` (role DRIVER) returns the route reserved to the authenticated driver: `{"id": "<routeId>", "stops": [...]}`, stops ordered by `sequence` starting at 1.
S2. Each stop has `id` (the order id), `sequence`, `address`, `windowStart`, `windowEnd` (HH:mm), `status` (PENDING, PICKED_UP, IN_TRANSIT, DELIVERED, NOT_DELIVERED), `reason` (null in this slice), and `latitude`/`longitude` when known.
S3. A driver with no route assigned gets `200` with `{"id": "", "stops": []}` (not 404, so the app does not show "service not available").
S4. Only the driver's own route is served. Other roles get `403`. A missing or unauthenticated caller gets `401`. The response never includes customer name, phone or other drivers' data.
S5. Stop status and window come from Order Service. The route is looked up from Driver Service by the driver's user id. Order Service refuses the stops list to any role other than DRIVER.
S6. Mobile: when there are no active stops, the "Activos" tab shows an empty state that says there is no active stop. When there are no completed stops, "Completados" shows its own empty state. When the driver has no route at all, the screen says no route is assigned today. The empty states are not an error.
S7. The stop result endpoint (`POST .../result`) and the other 501 driver stubs stay as they are. This slice does not change them.
S8. Contracts are additive only (`buf breaking` passes against the base).

User wording (verbatim, Spanish): "necesito que hagas que ya este disponible el servidor de rutasy funcione la paarte del repartidor en movil" and "la 1 me parece correcto tambien mejora la vista de cuando no hay rutas activas u  completadas".

## Tasks

- [x] T1 (S5, S8) contract: `ListDriverRouteStops` RPC and `DriverRouteStop` message (no customer identity). Evidence: `buf lint` exit 0; `buf breaking contracts --against ".git#ref=HEAD,subdir=contracts"` exit 0; `go build ./...` in contracts exit 0. Generated `gen/go/orders/v1` only.
- [x] T2 (S5) order-service. Use case `DriverRouteStops` (DRIVER gate, blank route check), port `DriverRouteStopRepository` (separate from `RouteStopRepository` so existing fakes keep compiling), persistence query ordered by `stop_sequence`, excludes CANCELLED (a cancelled order keeps its `stop_sequence`). gRPC handler `ListDriverRouteStops` (PERMISSION_DENIED for non-DRIVER, INVALID_ARGUMENT for blank route), wiring in `cmd/api/main.go`. Evidence: order-service full suite `go test -race ./...` all ok; `go build ./...` exit 0. Pending: integration test of the persistence query against a disposable DB (not run: the shared dev DB is truncated by those tests).
- [x] T2b (S5, S8) driver-service: `DriverSummary.route_id = 3` (additive) returned by `GetDriverByUserID`. Evidence: `buf lint`, `buf breaking` exit 0; driver-service `go test -race ./...` all ok (new test `TestGetDriverByUserIDIncluyeLaRutaReservada` RED then GREEN).
- [x] T3 (S1-S4) gateway: `GET /api/v1/drivers/me/route`. Handler `driverRouteHandlers.get` in `driver_route_handlers.go`, wrapper `Orders.ListDriverRouteStops`, router wired when Orders is set. Evidence: `driver_route_read_test.go` 7/7 PASS (RED first: 501 from the stub). Full gateway `go test -race ./...` all ok, `go vet` exit 0, `gofmt` clean. Decisions: no route or no driver profile gives `200 {"id":"","stops":[]}`; ROUTE_ASSIGNED reads as PENDING; `reason` is null.
- [x] T4 (S6) mobile: `ruta/index.tsx` shows an empty card for "Sin ruta asignada hoy" (route id empty) and per-tab empty cards for "No tienes paradas activas" and "Aún no completas paradas". Subtitle reads "Sin ruta asignada hoy" when there is no route. Evidence: `npm run typecheck` exit 0. Jest not run (environment: `jest-expo` preset fails on the missing babel config, see Progress of the mobile task).
- [ ] T5 verify: stack rebuilt (order-service, driver-service, api-gateway). Live without token: `GET /api/v1/drivers/me/route` answers 401 (route wired, was 501). Pending: live with a DRIVER token whose profile has a reserved route. Needs a driver account with credentials.

## Log

L1 (user, verbatim): "necesito que hagas que ya este disponible el servidor de rutasy funcione la paarte del repartidor en movil"
L2 (user, verbatim): "la 1 me parece correcto tambien mejora la vista de cuando no hay rutas activas u  completadas" (option 1 = read only; the result endpoint stays out).
L3 Finding: the 501 comes from `driverRouteNotImplemented` in the gateway router, not from routing-service, which is running. The stop status PATCH is already served.
L4 Assumption (to confirm): a driver with no route gets `200` with an empty route, not `404` (S3).

## Progress / evidence

- Feature created before any code change.
- Interrupted by a user request for web fixes (done, see below). The driver-route work resumes at T2 pending items.
- Web fixes (not part of this feature; branch `feat/seguimiento-cliente-rutas-web` in Frontend-Pikka): Zonas map fills the left column; the selected place and "Quitar selección" moved to the right panel. `tsc -b` exit 0; vitest zones+geo 134/134 PASS.
- Driver-service 500 on `GET /api/v1/drivers`: migrations 000004 and 000005 had never been applied to `last_mile` (columns missing, while schema_migrations said 7). Applied both in one transaction; backup at `%TEMP%\drivers_schema_backup_20261018.sql`. Not yet verified through the gateway (no admin token available to the agent); the user verifies in the browser.
- Not caused by this repo: console error "Search engine null is not supported" (`searchAnalyzer.js`). Not found in `apps/web/src`; likely a browser extension. `GET /api/v1/zones/classify` 404 is the expected "no zone covers this point" answer (gateway maps NOT_FOUND to 404); the UI handles it, but the browser logs it as an error. Changing it to 200 with `zone: null` is a contract decision, not done.
