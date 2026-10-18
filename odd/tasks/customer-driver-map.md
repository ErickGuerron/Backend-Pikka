# Feature: customer-driver-map

Branch: `feat/customer-driver-map` (from `feat/driver-route-read`). Route: parent writes inline per task. Delivery: slices of about 400 authored lines. Test runner: `go test -race -count=1 ./...` per Go module; `npm run typecheck` in the mobile app; Jest in the mobile app is blocked by the environment (see Progress).

## Specs

S1. While the driver's route screen is open and the route has at least one active stop, the driver app reports its position to `PUT /api/v1/drivers/me/location` every 15 seconds. Foreground only: no background tracking (ADR 0007).
S2. The app asks for location permission when the route screen opens. If the driver refuses, the screen says tracking is off and the rest of the screen keeps working.
S3. The customer snapshot `GET /api/v1/orders/{id}/tracking` includes `driverPosition: {latitude, longitude, recordedAt}` only when the driver's last position is fresh. Otherwise `driverPosition` is null. Assumption A1: fresh means recorded within the last 2 minutes.
S4. The snapshot never includes the driver's identity: no id, name, or phone.
S5. The customer's order screen shows a map with the delivery point and the driver's position, plus the distance in km and the ETA in minutes when they are available.
S6. Out of scope: optimal route ordering, route geometry, and the stop result endpoint (`POST .../result`, still 501).

User approval (verbatim, Spanish): "lo apruebo", answering the question about showing the driver's position on the customer's map with distance and ETA.

## Tasks

- [x] T1 (S1, S2) driver app: `put` added to the HTTP client; `reportDriverLocation` calls `PUT /api/v1/drivers/me/location`; `useDriverLocationReporter` (`features/driver-route/location-reporter.ts`) reports every 15 s while the route screen is open and there are active stops, requests foreground permission, and shows "Compartiendo tu ubicación" or the denied notice in `ruta/index.tsx`. Evidence: `npm run typecheck` exit 0. Not yet tested on a device; no unit test yet (Jest blocked).
- [x] T2 (S3, S4 at the source) tracking-service: `GetDriverPosition` RPC (contract additive: `buf lint`, `buf breaking` exit 0). Use case `FreshPosition` (ErrNoPosition / ErrStalePosition, window 120 s = assumption A1). gRPC: NOT_FOUND when missing or stale, INVALID_ARGUMENT for a blank id. TDD: RED first, then GREEN. Evidence: `go test -race ./...` all ok in tracking-service.
- [x] T3 (S3, S4) gateway: `driver_position` (snake_case, as the snapshot already is; the spec name `driverPosition` is adapted) and `delivery_point` added to the customer snapshot. Fresh position only, null otherwise; no driver identity (test checks identifiers, not the word "driver", since the key name contains it). Evidence: `go test -race ./...` all ok in api-gateway, `go vet` exit 0.
- [x] T4 (S5) customer app: `parseTrackingSnapshot` reads `driver_position` and `delivery_point`; `OrderTrackingSection` renders the existing Leaflet map (WebView) with a blue driver marker and the delivery point, fit to both; a note when the driver is known but there is no ETA. The map document gained a `driver` marker layer. Evidence: `npm run typecheck` exit 0. Jest not run (environment).
- [ ] T5 verify: Go suites for tracking-service and api-gateway, `buf lint`, `buf breaking`, typecheck, and a live check with the compose stack.

## Log

L1 (user, verbatim): "no aparece nada el mapa para que el cliente peuda ver el tracking en vivo del repartidor" and "tambien en la imagen de la vista del repartidor ... no se genera la ruta optima hasta que pongo en abrir ruta en el GPS".
L2 (user, verbatim): "lo apruebo".
L3 Finding: the driver app never calls `PUT /drivers/me/location` (0 call sites). The position is only captured when a result is registered, and that endpoint is 501. So the tracking has no data, and the ETA is null. Confirmed in code.
L4 Finding: `tracking-service` already has `RecordPosition` and `EstimateArrival`, but no RPC to read the last position of a driver.
L5 (user, verbatim): "la 1 necesito que lo bloques ... aunque tambien deberiamos ver el caso en que marquen mal una direccion dentro de una zona aceptable y el motorizado no pueda marcar como entregado por ello ya sea notificando a un operador o que el cliente reasigne la direccion mientras el pedido no este asignado a un motorizado".
Decision D1: a stop result outside the arrival radius (50 m) is blocked, not only recorded. Open case (D3): a wrong address inside an accepted zone; the operator is notified or the customer reassigns the address while the order is not assigned to a driver. Pending user choice.
L6 (user, verbatim): "la ruta tiene que mostrarse tanto en leaflet map como cuando agrega la opcion del gps". Finding: "Abrir ruta en el GPS" (`RouteSummary.tsx`) opens Google Maps with addresses only, in sequence order, with no OSRM and no coordinates. The in-app map shows numbered pins with no route line. Requirement: one OSRM route, the same order, in both places.
Open D2: may the driver reorder stops, and if so is the order persisted for everyone (changes the customer's stops_before) or only for the driver's view?

## Progress / evidence

- Feature document created before any code change.

L7 (user): "a". Decision D2: the driver may reorder stops; the order is persisted for everyone (the customer's stops_before changes) and the route is recalculated with OSRM.
Plan (in this order, each slice TDD, each with its own commit):
  P1 D1 + result endpoint: POST .../result served by the gateway; RegisterDelivery in driver-service rejects a result outside DRIVER_ARRIVAL_RADIUS_METERS (50 m) with FAILED_PRECONDITION; the order gets DELIVERED or NOT_DELIVERED.
  P2 OSRM route: libs/osrm returns the geometry (overview=full, geojson) and the optimized stop order (trip) for the driver's pending stops; exposed through tracking and the gateway.
  P3 Re-sequence: order-service persists the new stop_sequence for the route's pending stops; stops_before follows it.
  P4 Map: the same OSRM geometry is drawn in LeafletMap (driver and customer) and used to build the GPS link with coordinates in the same order.
  P5 Driver UI: drag or move-up control to pick the first stop; the change calls P3.
Open D3 (wrong address inside an accepted zone): operator notified, or customer reassigns the address while the order is not assigned to a driver. Not implemented until the user chooses.

L8 (user): "en este caso si se marca el motivo como direccion mal ingresada o direccion invalida, el operador debera confirmarlo". Decision D4: a surcharge for WRONG_ADDRESS (wrong or invalid address) waits for operator confirmation before it is charged. Other reasons keep the automatic rule of ADR 0006. Requires amending ADR 0006 (pending verification state) and ADR 0007 section 5. Not implemented yet.
Progress: ADR 0006 (section 1) and ADR 0007 (section 5) amended with PENDING_VERIFICATION for WRONG_ADDRESS (D4). Documentation only; no code changed in this step.
Progress P1 (part 1): D1 implemented in driver-service. Tests in internal/application/driver/delivery_radius_test.go: DELIVERED outside radius -> ErrOutsideRadius (FAILED_PRECONDITION), not recorded; DELIVERED inside accepted; NOT_DELIVERED at distance recorded. Old test TestRegisterDeliveryUsaElRadioConfigurado now uses NOT_DELIVERED (it asserted a far DELIVERED was accepted, which contradicts D1). Evidence: go test -race ./... ok in driver-service. Still open for P1: ON_ROUTE transition on first PICKED_UP (no code sets ON_ROUTE today) and the order outcome RPC in order-service; gateway POST .../result not wired yet.
Progress P1 (part 2): ON_ROUTE on first PICKED_UP. driver-service: StartRoute RPC (contract additive, buf lint/breaking ok), application StartRoute idempotent for the same route (start_route_test.go, 4/4 RED then GREEN), gRPC handler with INVALID_ARGUMENT for blank ids and FAILED_PRECONDITION for a mismatched route. api-gateway: DriverClient.StartRoute + wrapper; advance handler calls StartRoute before AdvanceRouteStop only when the target is PICKED_UP (driver_start_on_pick_test.go, 2/2). Evidence: go test -race ./... ok in driver-service and api-gateway. Still open for P1: order outcome RPC in order-service and POST .../result in the gateway.
