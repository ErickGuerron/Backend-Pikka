# order-service

Servicio que materializa el dominio de órdenes (esquema `orders`) y, en esta
primera entrega, la entidad de negocio **Customer** del MVP. Expone HTTP para
health checks y gRPC para el dominio Customer (BE-T5a); BE-T5b suma los
endpoints REST del API Gateway que consumen este server gRPC.

## Estado (BE-T5a — CustomerService gRPC)

Este commit publica el `CustomerService` del feature `customer-mvp-backend`
dentro de `order-service`:

- Contrato en `contracts/proto/customer/v1/customer.proto` con cuatro rpcs:
  `CreateCustomer`, `GetByUserID`, `UpdateStatus`, `CanCreateOrders`.
- Server gRPC en `internal/adapters/grpc/customer_server.go` que solo
  traduce: parsea entradas, delega en los use cases de `internal/application/customer`
  y mapea los sentinel errors del dominio a códigos gRPC
  (`NotFound`, `AlreadyExists`, `InvalidArgument`, `Internal`, etc.).
- Listener gRPC en `cmd/api/main.go` con `GracefulStop` en el shutdown,
  configurable vía `ORDER_GRPC_PORT` (default `9091`).
- Tests unitarios en `internal/adapters/grpc/customer_server_test.go` con un
  `fakeCustomerRepository` in-memory que cubre happy paths y los errores de
  cada rpc.

## Estado previo (BE-T4b — dominio Customer)

- Dominio `Customer` con tres estados (`ACTIVO`, `BLOQUEADO_POR_SALDO`,
  `BLOQUEADO_POR_INCIDENCIAS`) y el acumulador `pending_surcharge`
  (`NUMERIC(12,2)` → `decimal.Decimal` en Go).
- Cuatro casos de uso (`CreateCustomer`, `GetByUserID`, `UpdateStatus`,
  `CanCreateOrders`) con tests unitarios sobre un repositorio in-memory.
- Repositorio PostgreSQL con `pgxpool`, manejo de `ErrNoRows` y traducción
  de unique violation a `ErrCustomerAlreadyExists`.
- Migración `000001_create_customers` con `UNIQUE INDEX` sobre `user_id`
  (ADR 0003) y CHECK constraints que protegen la integridad a nivel de DB.

## Estructura

```
services/order-service/
├── cmd/api/                    # entrypoint (slog + net/http + grpc)
├── internal/
│   ├── adapters/
│   │   ├── grpc/               # CustomerServiceServer (BE-T5a)
│   │   ├── http/               # /health, /ready (BE-T4a)
│   │   └── persistence/        # CustomerRepository (BE-T4b)
│   ├── application/
│   │   ├── customer/           # casos de uso + tests (BE-T4b)
│   │   └── ports/              # CustomerRepository interface
│   ├── domain/                 # Customer, CustomerStatus, errors (BE-T4b)
│   └── infrastructure/
│       ├── config/             # env loader (HTTP_PORT, ORDER_GRPC_PORT)
│       └── postgres/           # pool + migrador
└── migrations/
    ├── 000001_create_customers.up.sql
    ├── 000001_create_customers.down.sql
    └── embed.go
```

## Modelo

- Esquema `orders` (creado por `infrastructure/postgres/init/01-schemas-and-users.sh`).
- Usuario `order_service_user`, dueño del esquema.
- Tabla `customers` con PK UUID, `UNIQUE INDEX` sobre `user_id`, `email`
  único, `pending_surcharge NUMERIC(12,2)` y CHECK constraints
  (status válido, email en minúsculas, saldo no negativo).

## Variables de entorno

| Variable             | Default | Descripción                                   |
| -------------------- | ------- | --------------------------------------------- |
| `HTTP_PORT`          | `8080`  | Puerto HTTP para `/health` y `/ready`.        |
| `ORDER_GRPC_PORT`    | `9091`  | Puerto gRPC para `CustomerService`.           |
| `POSTGRES_PASSWORD`  | —       | Requerida. Sin valor por defecto (credencial). |
| `POSTGRES_HOST`      | `localhost` | Host de PostgreSQL.                       |
| `POSTGRES_PORT`      | `5432`  | Puerto de PostgreSQL.                         |
| `POSTGRES_DB`        | `last_mile` | Base de datos.                            |
| `POSTGRES_USER`      | `order_service_user` | Usuario.                            |
| `POSTGRES_SCHEMA`    | `orders` | Esquema del servicio.                        |

## Pruebas

```bash
go test ./...
```

Cobertura de `internal/domain` al 100 %. Los tests de casos de uso y del
adapter gRPC corren contra fakes in-memory; los tests de integración del
`CustomerRepository` de PostgreSQL se agregan junto con BE-T5b.
