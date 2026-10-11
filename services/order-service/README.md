# Order Service

Base del Sprint 1 de Anthony (S1-BE-02): dominio de pedidos, fechas y ventanas
de entrega, estados y migraciones PostgreSQL. Es un módulo Go independiente
incluido en `go.work`, `make test`, `make lint` y CI.

El workspace y este módulo recomiendan el toolchain Go 1.26.9 para incluir los
parches de seguridad de la biblioteca estándar. Go puede descargarlo de forma
automática; no requiere reemplazar la instalación global.

El job Go de este módulo usa `GOWORK=off` en CI. Así comprueba que las
dependencias están declaradas en su propio `go.mod`, sin heredar versiones de
otros servicios a través del workspace.

El contrato `orders.v1` está en `contracts/proto/orders/v1/orders.proto` y su
correspondencia REST en `docs/contracts/orders-v1.md`. El transporte gRPC y su
consumo desde el Gateway pertenecen al Sprint 2. Esta base todavía no levanta
un servidor ni habilita los endpoints de pedidos.

## Reglas del dominio

- `NewOrder` recibe datos de entrega y un instante del reloj del servidor en la
  zona horaria de operación. Guarda `CreatedAt` en UTC y conserva `DeliveryDate`
  como fecha civil, incluso cuando el día UTC sea distinto del día local.
- La entrega puede programarse hoy o en el futuro. Se rechazan fechas inexistentes
  y formatos distintos de `YYYY-MM-DD`.
- Las ventanas usan `HH:mm`; el fin es posterior al inicio en el mismo día.
- La dirección se recorta y tiene entre 5 y 300 caracteres Unicode, sin NUL
  (PostgreSQL no puede almacenar ese carácter en texto). Las coordenadas
  WGS 84 deben ser finitas y estar dentro de sus rangos.
- Un pedido nuevo comienza en `PENDING`. El ID y la versión los asigna la base.
  Las transiciones devuelven una copia y no alteran el pedido original.

| Estado | Siguientes estados permitidos |
|---|---|
| PENDING_REVIEW | PENDING, CANCELLED (rechazo del operador) |
| PENDING | ROUTE_ASSIGNED, CANCELLED |
| ROUTE_ASSIGNED | PICKED_UP, CANCELLED |
| PICKED_UP | IN_TRANSIT |
| IN_TRANSIT | DELIVERED, NOT_DELIVERED |
| DELIVERED, NOT_DELIVERED, CANCELLED | Ninguno |

`CanCancel` permite la cancelación ordinaria únicamente en `PENDING` y
`ROUTE_ASSIGNED` (ADR 0007). La autorización, revisión de excepciones, motivos
de auditoría y resultado de entrega se implementan en sus casos de uso de los
sprints posteriores. Los estados correspondientes ya son reconocidos.

## Persistencia

Las migraciones embebidas crean `orders.orders` con `DATE` para la entrega,
`TIMESTAMPTZ` para el registro, ventanas `TIME`, estado, motivos de revisión y
versión positiva. PostgreSQL valida coordenadas, estados, ventanas y coherencia
entre `PENDING_REVIEW` y sus motivos. Hay índices por fecha y estado, y por
instante de registro. La migración inversa elimina la tabla y sus índices.
Una segunda migración limita los años de entrega a 0001–9999, para impedir que
PostgreSQL acepte fechas anteriores a nuestra era o años de cinco dígitos que el
contrato `YYYY-MM-DD` no representa. Incluye su propia migración inversa.

`internal/infrastructure/postgres.Migrate` aplica migraciones pendientes con
`golang-migrate` y acepta una segunda ejecución sin cambios. El DSN debe usar
`order_service_user` y `search_path=orders,public`. No hay accesos ni claves
foráneas hacia esquemas de otros servicios. La relación `customer_id` está
prevista para la migración del Sprint 2.

## Verificación

Desde este directorio:

```bash
go test -coverprofile=coverage.out ./...
go vet ./...
golangci-lint run --config ../../.golangci.yml ./...

# Base local inicializada por el compose, usando la contraseña de tu .env.
export ORDER_TEST_DATABASE_URL="postgres://order_service_user:${ORDER_DB_PASSWORD}@127.0.0.1:5432/last_mile?sslmode=disable&search_path=orders,public"
go test -race -tags integration -count=1 ./...
```

En PowerShell, asigna la variable con `$env:ORDER_TEST_DATABASE_URL = '...'` y
usa `go test '-coverprofile=coverage.out' ./...`. `-race` en Windows requiere
un compilador C; también puede ejecutarse en un contenedor Go de Linux.

Sin `ORDER_TEST_DATABASE_URL`, las pruebas de integración se omiten de forma
explícita. Con una base disponible verifican aplicación repetida, valores por
defecto, aislamiento por esquema, restricciones SQL, índices y reversibilidad.
Las operaciones de prueba se revierten con transacciones, incluidos los scripts
down y up, para conservar los datos preexistentes.

Evidencia y flujo de revisión: `docs/planning/sprint-1-anthony.md`.
