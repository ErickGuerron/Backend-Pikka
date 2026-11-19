# Feature: osrm-ecuador

Branch: `feat/osrm-ecuador`, stacked on `feat/routing-plan-generation` (the routing module is
not on `develop` yet).

## Objective

Run OSRM with the Ecuador road network (MVP) and expose a road distance and driving time
estimate between two points. This is the input order tracking needs: driver position to
the customer's address (ETA and distance).

## Scope

- In: compose service `osrm` (profile `osrm`); script that downloads the Geofabrik extract
  and runs extract, partition and customize; `make` targets; `routing-service` adapter
  `internal/adapters/osrm`; domain types `Point`, `Estimate`, `ErrNoRoute`, `ErrInvalidPoint`.
- Out: the tracking endpoint and the driver's live position (Tracking Service, Sprint 5),
  the gateway route, geometry (`overview=false`), caching, other regions.

## Constraints

- Map data is generated, large and not versioned (`infrastructure/osrm/data/`, git-ignored).
- OSRM image pinned to `osrm/osrm-backend:v5.25.0`. `v5.27.1` and `v5.26.0` are not published.
- Port bound to `127.0.0.1:5000` like the other local services.
- TDD strict. Unit tests with `httptest`; integration tests behind the `integration` tag and
  `OSRM_URL`, run against the real server.

## Design decisions

- Coordinates are sent as `lon,lat` (OSRM order). Invalid coordinates (out of range, NaN,
  infinite) fail before any HTTP call.
- `radiuses=1000;1000`: OSRM snaps any point to the nearest road with no limit by default. The
  integration test caught it: Bogotá (outside the map) returned a route, about 370 km snapped
  to the Ecuador border. With the radius, OSRM answers `NoSegment` (HTTP 400).
- `NoRoute` and `NoSegment` are both `domain.ErrNoRoute`. The body code decides before the HTTP
  status, because OSRM returns 400 for `NoSegment`.
- `;` is written literally in the query. `url.Values` escapes it to `%3B`, which the server
  accepted in the test but the documented form is the literal separator.
- The timeout is per call (`http.Client.Timeout`) and the context is respected.

## Tasks

- [x] T1 Infra: `docker-compose.yml` service `osrm` (profile `osrm`), `.gitignore` for the data
      directory, `scripts/osrm-prepare-ecuador.ps1`, `make osrm-prepare|up|down|test`.
      Check: `docker compose --profile osrm config` valid; `make osrm-prepare` exit 0 (extract
      192 s, partition 17 s, customize 2 s, peak RAM about 1.9 GB); `make osrm-up` healthy.
- [x] T2 Domain types and errors (TDD). Route: inline, the adapter is 1 file.
- [x] T3 Adapter (TDD): RED = undefined `NewEstimator`, then `radiuses` and HTTP 400
      `NoSegment` missing, each seen failing before the fix. Evidence in the unit run.
- [x] T4 Integration against the real server: Quito route 6283 m / 653 s (OSRM); Bogotá gives
      `ErrNoRoute` after the radius fix (it was a route before the fix: the RED that justified it).
- [x] T5 Lint: `golangci-lint` 0 issues; `go vet` clean; `gofmt` clean.

## Acceptance criteria

- `make osrm-prepare` produces the data from the Geofabrik extract without manual steps.
- `make osrm-up` serves `route/v1/driving` on `127.0.0.1:5000`.
- `make osrm-test` passes against that server; the integration tests run (not SKIP).
- A point off the map is `ErrNoRoute`, never a route.

## Progress / evidence

- Unit: 9 tests in `internal/adapters/osrm`, `-race`, ok.
- Integration (real OSRM v5.25.0, Ecuador extract of 125,634,003 bytes): 2 tests, `-race`, ok.
- Routing module: all packages ok under `-race`; golangci-lint 0 issues.

## Next step

Tracking Service consumes `Estimator.Estimate` with the driver's last known position and the
order's coordinates. Not started: it needs the tracking module, the status transitions
(`PICKED_UP`, `IN_TRANSIT`, `DELIVERED`) and the Redis last-position store.
