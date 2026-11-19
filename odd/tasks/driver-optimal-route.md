# Feature: driver-optimal-route

Branch: not created yet (working tree is on `docs/adr-wrong-address-verification`; open
`feat/driver-optimal-route` from `develop` before committing). Nothing is committed.
Frontend part lives in `Frontend-Pikka/apps/mobile` on the same feature name.

## Objective

The driver can have several active orders. The route screen draws the best visiting order of
those stops on the Leaflet map, following the road (OSRM trip service). The driver can fix the
order to attend, and a button re-centers the map on that order and the next point.

## Specs

- S1: "dibujar la ruta mas optima dentro del leaftlet map": the route is drawn on the map along
  the roads, in the optimal order of the active stops.
- S2: "el repartidor puede tener mas pedidos activos y que el puede fijar el pedido a atender":
  the driver chooses which active order to attend; that choice is the point the map centers on.
- S3: "necesito un boton para re encuadrar en el punto del pedido y el punto a seguir": a button
  fits the map to the pinned order's point and the next point to follow.
- Assumption A1 (confirm later): fixing an order is stored only on the phone, not visible to the dispatcher.
- Assumption A2: the origin of the optimal route is the phone's current GPS position, sent by
  the app as query parameters (`lat`, `lon`), because Tracking's proto has no position read yet.
- Assumption A3: OSRM is reached only through the API Gateway; the phone never calls OSRM directly.

## Scope

- In: `libs/osrm` trip (solver) client; gateway driver-only endpoint; compose wiring for the
  OSRM URL; mobile map polyline, order pin, re-center button.
- Out: dispatcher view of pinned orders; persisting the pin; `routing-plan-generation` (backend
  planning by zone); background tracking.

## Tasks

- [x] T1 Shared OSRM trip client (TDD): `libs/osrm/trip.go` `Tripper.Optimize(origin, stops)`
      returns visiting order (indexes into stops), totals and geometry. RED: undefined
      `NewTripper`; GREEN: 6 unit tests with `httptest`. Check: `go test -race ./...` in
      `libs/osrm` ok; `go vet` and `gofmt` clean. Integration against the real server not run
      (needs `make osrm-up`, Docker).
- [x] T2 Gateway endpoint `GET /api/v1/drivers/me/route/plan?lat=&lon=`, role DRIVER. Returns
      `{ order: [stopId...], distanceMeters, durationSeconds, geometry: [[lat,lon]...] }`.
      Open: confirm the driver route response carries stop coordinates; without them the
      endpoint cannot optimize and must say so (not guess). Check: gateway handler tests; 403/401
      cases like the other driver routes.
- [x] T3 Compose and config: `OSRM_URL` for the gateway, `osrm` profile dependency documented.
      Check: `docker compose config` valid.
- [x] T4 Mobile: fetch the plan, draw the polyline and numbered stops in optimal order on
      `LeafletMap` (`state.stops` plus a route line). Check: `route-flow` and map tests (RED first).
- [x] T5 Mobile: pick the order to attend (default: first of the optimal order), kept in screen state.
      Check: RED test that the pinned order drives the "current" card and the map focus.
- [x] T6 Mobile: "Re-encuadrar" button fits the map to the pinned order point and the next point.
      Needs a fit request in the map contract (a nonce field, not a focus that moves on every render).
      Check: RED test on the document contract plus the screen test.
- [ ] T7 Verify: mobile suite and typecheck; backend `go test -race` per touched module; live check
      with `make osrm-up` and a DRIVER token once T2 exists.

## Progress / evidence

- T2 (TDD): 7 gateway tests in driver_route_plan_test.go, RED = undefined RoutePlanner; full gateway suite -race ok.
- T3: gateway go.mod replace libs/osrm, Dockerfile copies libs/osrm, OSRM_URL in compose (empty disables the endpoint, 503). go build, go vet, gofmt clean.
- T4-T6 (mobile): plan.ts, attend.ts, plan-hooks.ts, map-state.ts (road, fit), document.ts (road layer, fit by key), RouteCard re-encuadrar button, ruta screen pin button. Helper tests in driver-optimal-route.test.ts and map-document.test.ts. Full mobile suite 19 suites / 185 tests; tsc clean.
- Not covered by automated tests yet: screen-level pin and re-encuadrar interaction, OSRM_URL in config_test, the trip integration test against real OSRM.

## Progress / evidence (older)

- T1: `go test -race ./...` in `libs/osrm`: ok. `go vet`, `gofmt -l`: clean.

## Next step

T2: confirm stop coordinates in the driver route response, then write the gateway handler tests (RED).

