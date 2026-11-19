# zone-service

Gestiona las zonas de reparto y clasifica coordenadas. Contrato: `contracts/proto/zones/v1/zones.proto`.

## Qué hace

- `CreateZone`: registra una zona definida por un anillo cerrado de coordenadas.
- `GetZone`: devuelve una zona por su identificador.
- `ClassifyPoint`: devuelve la zona que contiene una coordenada. Si varias la contienen, devuelve la de menor área. Responde `NOT_FOUND` si ninguna la contiene.

## Modelo

- Tabla `zones` en el esquema `zones`, con área `geography(POLYGON, 4326)`.
- Índice GiST sobre `area` (`zones_area_gist`) para acelerar la clasificación.
- La clasificación usa `ST_Covers`, que incluye los puntos sobre el borde.

## Pruebas

```bash
go test ./...                                    # unitarias y gRPC
ZONE_TEST_DATABASE_URL=postgres://zone_service_user:<password>@127.0.0.1:5432/last_mile?sslmode=disable&search_path=zones,public \
  go test -tags=integration ./internal/adapters/persistence/...
```

Sin `ZONE_TEST_DATABASE_URL`, las pruebas de integración se omiten. La base debe tener PostGIS en `public`, por eso `search_path` incluye `public`.
