# MVP — Customer backend: auth CUSTOMER + dominio cliente

**Owner:** Mabe (en este caso también backend, decisión del usuario).
**Base normativa:** ADR 0006 (recargo), ADR 0007 (alcance MVP), ADR 0008 (permisos por rol).
**Branch de trabajo:** `feature/customer-mvp-backend` (a crear desde `origin/develop`).
**Tipo de cambio:** sustancial (nuevo servicio gRPC + nueva tabla + migración + endpoints).

## Objetivo

Que un cliente pueda registrarse y autenticarse desde `apps/mobile` (eso es Auth Service), y que el resto del sistema pueda administrarlo como `Customer` con estado de cuenta, saldos por cobrar e incidencias (eso es dominio nuevo).

## Por qué

ADR 0007 pide "Auth Service agrega el rol `CUSTOMER`" y ADR 0006 pide saldos pendientes que pueden bloquear la cuenta. Estos dos efectos requieren código: el rol solo en el enum no alcanza, hay que representar el dominio del cliente.

## Alcance

### Incluido

- `ROLE_CUSTOMER` en el enum y migración del CHECK constraint.
- `SignupCustomer` en `AuthService` gRPC.
- `POST /api/v1/auth/signup` en el Gateway con rate limit.
- `GET /api/v1/users` (solo ADMIN).
- Nueva tabla `customers` en esquema `orders` con estado `ACTIVO`, `BLOQUEADO_POR_SALDO`, `BLOQUEADO_POR_INCIDENCIAS`.
- Nuevo servicio `CustomerService` gRPC.
- Endpoints REST: `GET /api/v1/customers/me`, `GET /api/v1/customers`, `PATCH /api/v1/customers/{id}/status`.
- Tests de BOLA, de signup y de transición de estado de cliente.

### NO incluido (queda fuera)

- `payment_attempts`, `PaymentGateway` simulado, hook del 15%. Eso es un feature aparte.
- WebSocket de tracking (Fase 6).
- Flujo del repartidor en backend (eso es Driver Service de Mabe, ya plan aparte).
- Generación de pedidos desde CUSTOMER (eso depende de Order Service ampliar; ver feature doc Tobile).

## Restricciones

- TDD: por cada tarea RED → GREEN → REFACTOR. Tests primero.
- ADR 0008 §3 del archivo: rol → `403` sin consultar DB; alcance → filtro por `user_id` antes de tocar la tabla.
- El dominio `Customer` es una entidad de **negocio**, no de auth. La fila `users` existe en el esquema `auth`; la fila `customers` existe en el esquema `orders` y referencia `users.id` por FK.
- `pending_surcharge` es NUMERIC(12,2) para evitar drift de floats. Cualquier UPDATE pasa por gRPC.

## Estructura objetivo

```text
services/auth-service/
├── internal/
│   ├── domain/
│   │   ├── role.go (NUEVO valor CUSTOMER)
│   │   └── user_test.go (NUEVOS tests de SignupCustomer)
│   └── adapters/grpc/
│       └── server.go (NUEVA rpc SignupCustomer)
├── migrations/
│   └── 000002_add_role_customer.up.sql / .down.sql (NUEVO)

services/order-service/  (NUEVO servicio, antes no existía)
├── internal/
│   ├── domain/
│   │   ├── customer.go (NUEVO)
│   │   ├── customer_status.go (NUEVO)
│   │   └── customer_test.go (NUEVO)
│   └── adapters/
│       ├── grpc/customer_server.go (NUEVO)
│       └── persistence/customer_repository.go (NUEVO)
├── migrations/
│   └── 000001_create_customers.up.sql / .down.sql (NUEVO)

services/api-gateway/
├── internal/adapters/grpcclient/customer.go (NUEVO)
├── internal/adapters/http/
│   ├── auth_handlers.go (NUEVO handler signup)
│   ├── customer_handlers.go (NUEVO)
│   └── router.go (NUEVAS rutas)
└── contracts/
    ├── proto/auth/v1/auth.proto (NUEVA rpc SignupCustomer)
    ├── proto/customer/v1/customer.proto (NUEVO)
    └── proto/buf.yaml (registrado customer/v1)
```

## Tareas (orden de ejecución)

### BE-T1 — Ampliar el enum con `ROLE_CUSTOMER` + migración

**Qué:** Agregar `RoleCustomer Role = "CUSTOMER"` a `services/auth-service/internal/domain/role.go`. Crear migración `000002_add_role_customer.up.sql` que suelte el CHECK `users_role_valid` y lo recree incluyendo `'CUSTOMER'`. Tests: `Role("CUSTOMER").Valid()` → true, `Role("INVALID").Valid()` → false (regresión).

**Por qué:** Sin esto, ninguna de las tareas siguientes funciona. Es la base.

**Entregable:** `role.go`, dos archivos de migración, dos tests pasando.

**Verificación:** `go test ./...` en `auth-service` verde.

### BE-T2 — `SignupCustomer` en AuthService + `POST /api/v1/auth/signup`

**Qué:** En `auth.proto`, agregar al servicio `AuthService` la rpc `SignupCustomer(SignupCustomerRequest) returns (SignupCustomerResponse)`. Implementarla en `server.go` con validación de email único, hash de contraseña (mismo algoritmo que `CreateUser`), y emisión de JWT con rol `CUSTOMER`. Exponer `POST /api/v1/auth/signup` en el Gateway **sin middleware JWT** pero con rate limit (5 req/IP/min). OpenAPI actualizado con el nuevo endpoint.

**Por qué:** El frontend móvil consume este endpoint. Sin él el cliente no puede registrarse.

**Entregable:** proto actualizado + buf gen + server.go + handler http + ruta en router + OpenAPI + tests:
- email duplicado → 409
- email inválido → 400
- contraseña < 8 caracteres → 400
- signup válido → 201 con `accessToken`
- `GET /api/v1/auth/me` con ese token → `role: "CUSTOMER"`

**Verificación:** `make test`, `make openapi-lint`, `make lint`.

### BE-T3 — `GET /api/v1/users` (solo ADMIN)

**Qué:** El endpoint listar usuarios está mencionado en OpenAPI como faltante (ADR 0008 §4). Agregar `rpc ListUsers(ListUsersRequest) returns (ListUsersResponse)` a `AuthService`, handler en el Gateway, ruta protegida con `ADMIN`. ADR 0008 §3: `403` si el rol no es ADMIN, **antes** de tocar la base.

**Por qué:** El operador necesita ver la lista de clientes para atender la cola de excepciones.

**Entregable:** proto + server.go + handler + ruta + OpenAPI + tests de autorización.

**Verificación:** `make test` con test de BOLA incluido.

### BE-T4 — Crear `services/order-service` con Customer

**Qué:** Servicio Go nuevo siguiendo el patrón de los demás (mismo Dockerfile, mismo Makefile, integración en go.work y docker-compose.yml). Dominio:
- `CustomerStatus` enum: `ACTIVO`, `BLOQUEADO_POR_SALDO`, `BLOQUEADO_POR_INCIDENCIAS`.
- `Customer` struct con `ID`, `UserID`, `Status`, `PendingSurcharge`, `CreatedAt`, `UpdatedAt`.
- `Customer.CanCreateOrders()` que devuelve `false` si status != ACTIVO.
- Migración `000001_create_customers.up.sql` con FK a `users.id`, índices por `UserID` y `Status`.

**Por qué:** Sin este servicio no hay donde guardar el estado del cliente ni el saldo pendiente.

**Entregable:** servicio nuevo, dominio + tests al 100% de cobertura, migración aplicada, integrado en docker-compose.

**Verificación:** `go test ./...` verde, migración aplicada, health check del servicio verde.

### BE-T5 — `CustomerService` gRPC + endpoints REST

**Qué:** `contracts/proto/customer/v1/customer.proto` con:
- `rpc GetByUserID(GetByUserIDRequest) returns (GetByUserIDResponse)` — usado por Order Service para saber si el cliente puede pedir.
- `rpc UpdateStatus(UpdateStatusRequest) returns (UpdateStatusResponse)` — solo ADMIN.
- `rpc List(ListRequest) returns (ListResponse)` — para el operador.

REST en el Gateway:
- `GET /api/v1/customers/me` → CUSTOMER autenticado ve su propio `Customer`.
- `GET /api/v1/customers` → OPERATOR/ADMIN.
- `PATCH /api/v1/customers/{id}/status` → solo ADMIN.

ADR 0008 §3: rol → 403 antes de DB; alcance → filtro por `user_id` para `me`; `404` (no 403) si el recurso es de otro cliente.

**Por qué:** Es la API que va a invocar el frontend y los demás servicios.

**Entregable:** proto + buf gen + handlers + rutas + OpenAPI + tests:
- CUSTOMER A pide `me` → 200 con su Customer.
- CUSTOMER A pide `GET /customers/{id-de-B}` → 404.
- DRIVER pide `me` → 403.
- OPERATOR pide `GET /customers` → 200.
- ADMIN hace `PATCH` → 200; OPERATOR → 403.

**Verificación:** `make test`, `make openapi-lint`, `make security-scan` sin FAIL nuevos.

### BE-T6 — PR a develop + verificación final

**Qué:** PR único con todos los commits BE-T1 a BE-T5. Checklist:
- `make test`, `make test-integration` verdes.
- `make lint`, `make openapi-lint` verdes.
- `make security-scan` sin FAIL nuevos (los WARN preexistentes se documentan en el reporte).
- `make perf-smoke` con los nuevos endpoints: signup < 200ms p95, listado de customers < 100ms p95.
- Revisor: persona distinta de Mabe (regla del plan).

**Entregable:** PR mergeado a `develop`.

**Verificación:** CI verde + revisor aprobó.

## Dependencias

| Qué | Quién | Estado |
|---|---|---|
| Nada bloqueante en backend. | — | Pueda. |
| Dependiente de FE: `apps/mobile` puede consumir `/auth/signup` y `/customers/me` después de BE-T5. | Mabe (mobile) | Sigue su plan aparte. |
| Dependiente de Order Service para crear pedidos como CUSTOMER. | Anthony/Erick | No incluido en este feature. |

## Criterios de aceptación

- [ ] Un CUSTOMER puede registrarse y obtener un JWT con `role: "CUSTOMER"`.
- [ ] `GET /api/v1/auth/me` con ese JWT devuelve `role: "CUSTOMER"`.
- [ ] CUSTOMER A no ve el `Customer` de CUSTOMER B (BOLA probado, devuelve 404).
- [ ] OPERATOR lista customers; CUSTOMER no (403).
- [ ] ADMIN patchea el status; OPERATOR no (403).
- [ ] `services/order-service` corre, su health responde, su migración está aplicada.
- [ ] `make security-scan` sin FAIL nuevos.
- [ ] PR revisado por persona distinta de Mabe.

## Riesgos

- **Servicio nuevo**: `order-service` no existía. Crearlo desde cero es más riesgo que extender uno existente. Plan: clonar la estructura de `zone-service` y ajustar.
- **FK a `auth.users` desde `orders.customers`**: el usuario de `orders` no puede leer el esquema `auth` por permisos de la base (verificado en `permission denied for schema zones`). El JOIN debe hacerse por gRPC, no por SQL directo. Esto ya está contemplado: `GetByUserID` recibe el ID y hace su propia consulta; la unicidad se mantiene por el dominio, no por FK explícita en SQL (usamos `user_id` con UNIQUE INDEX, no FOREIGN KEY constraint).
- **Recargo**: este feature **no** aplica recargos todavía. Solo deja el campo `pending_surcharge` listo para que otro lo escriba.

## Commits de trabajo

| Tarea | Tipo | Mensaje prefijo |
|---|---|---|
| BE-T1 | feat | `feat(auth): agregar ROLE_CUSTOMER y migración del CHECK constraint` |
| BE-T2 | feat | `feat(auth): signup público de cliente con rate limit` |
| BE-T3 | feat | `feat(auth): listado de usuarios solo ADMIN` |
| BE-T4 | feat | `feat(orders): nuevo servicio con dominio Customer y migraciones` |
| BE-T5 | feat | `feat(orders+gateway): CustomerService gRPC y endpoints REST` |
| BE-T6 | chore | `chore: PR a develop con checklist de aceptación` |

## Convenciones heredadas

- ADR 0002 (JWT en Gateway, autorización por servicio).
- ADR 0003 (esquema y usuario por servicio, sin permisos cruzados).
- Sección 23 de la base técnica (estructura de features en frontend, pero aplica para módulos paralelos en backend).
- Convención de commits: Conventional Commits, sin "Co-Authored-By".