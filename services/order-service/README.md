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
correspondencia REST en `docs/contracts/orders-v1.md`. El transporte gRPC de
pedidos y su consumo desde el Gateway pertenecen al Sprint 2. El servidor
existente ya publica el dominio Customer y los health checks del servicio.

## Clientes del MVP

Se conserva la implementación integrada en `develop`: dominio `Customer`,
casos de uso en `internal/application/customer`, repositorio PostgreSQL y
`CustomerService` gRPC consumido por el Gateway. Los clientes tienen estados
`ACTIVO`, `BLOQUEADO_POR_SALDO` y `BLOQUEADO_POR_INCIDENCIAS`, con saldo
`pending_surcharge` representado por `decimal.Decimal`.

`cmd/api/main.go` aplica las migraciones del esquema, abre el pool y atiende HTTP
y gRPC. `HTTP_PORT` vale 8080 y `ORDER_GRPC_PORT` 9091 por defecto. La conexión
usa `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`, `POSTGRES_USER`,
`POSTGRES_SCHEMA` y la credencial requerida `POSTGRES_PASSWORD`.

La migración existente `000001_create_customers` se conserva. La creación de
pedidos usa la versión 2 y el límite de fechas la versión 3, para que las bases
que ya tienen clientes puedan actualizarse sin colisiones de versión.

## Recargos, saldo y bloqueo (ADR 0006)

Order Service lleva el motor de penalizaciones: recargo del 15 % al no entregar por culpa del cliente,
verificación del operador para `WRONG_ADDRESS`, reincidencia, cobro simulado (`PaymentGateway`, sin datos de
tarjeta), bloqueo por saldo vencido y pago del saldo. Las reglas, dónde viven en el código y lo que falta
están en `docs/adr/0006-recargo-por-rechazo-con-cobro-simulado.md` (sección 7); la correspondencia REST/gRPC en
`docs/contracts/orders-v1.md`.

`cmd/api/main.go` arranca además un barrido que primero recupera los recargos que no se registraron al cerrar una parada
(`ReconcileIncidents`) y después bloquea las cuentas con un saldo de más de 14 días: corre al
iniciar y cada `BALANCE_SWEEP_MINUTES` minutos (15 por defecto). Un fallo se registra en el log y el siguiente
pase reintenta.

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
La migración `000003_limit_delivery_date` limita los años de entrega a 0001–9999, para impedir que
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

# Con -p 1 los paquetes no corren en paralelo (la prueba de migraciones baja y sube el esquema).
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

Las pruebas obligatorias del ADR 0006 (recargo exacto y una sola vez, cobro siempre simulado, sin datos de
tarjeta en logs, respuestas ni tablas) corren con el resto: busque `ADR0006` en los nombres. Con una base disponible
se ejecutan también las de integración; sin `ORDER_TEST_DATABASE_URL` se omiten.

Evidencia y flujo de revisión: `docs/planning/sprint-1-anthony.md`.
