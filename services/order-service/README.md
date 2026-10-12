# order-service

Servicio que materializa el dominio de órdenes (esquema `orders`) y, en esta
primera entrega, la entidad de negocio **Customer** del MVP. Por ahora expone
únicamente HTTP para health checks; los endpoints gRPC de Customer entran en
BE-T5.

## Estado (BE-T4b — dominio Customer)

Este commit implementa el corazón de la sección del feature
`customer-mvp-backend` dentro de `order-service`:

- Dominio `Customer` con tres estados (`ACTIVO`, `BLOQUEADO_POR_SALDO`,
  `BLOQUEADO_POR_INCIDENCIAS`) y el acumulador `pending_surcharge`
  (`NUMERIC(12,2)` → `decimal.Decimal` en Go).
- Cuatro casos de uso (`CreateCustomer`, `GetByUserID`, `UpdateStatus`,
  `CanCreateOrders`) con tests unitarios sobre un repositorio in-memory.
- Repositorio PostgreSQL con `pgxpool`, manejo de `ErrNoRows` y traducción
  de unique violation a `ErrCustomerAlreadyExists`.
- Migración `000001_create_customers` que reemplaza los placeholders vacíos
  de BE-T4a. Crea la tabla `customers`, su `UNIQUE INDEX` sobre `user_id`
  (por ADR 0003) y los CHECK constraints que protegen la integridad a
  nivel de DB.

BE-T4a queda sin efecto en migraciones (los placeholders no creaban objetos);
el esquema se materializa ahora de forma atómica en una sola migración.

## Estructura

```
services/order-service/
├── cmd/api/                    # entrypoint (slog + net/http para BE-T4a)
├── internal/
│   ├── adapters/
│   │   ├── http/               # /health, /ready (BE-T4a)
│   │   └── persistence/        # CustomerRepository (BE-T4b)
│   ├── application/
│   │   ├── customer/           # casos de uso + tests (BE-T4b)
│   │   └── ports/              # CustomerRepository interface
│   ├── domain/                 # Customer, CustomerStatus, errors (BE-T4b)
│   └── infrastructure/
│       ├── config/             # env loader
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

## Pruebas

```bash
go test ./...
```

Cobertura de `internal/domain` al 100 %. Los tests de casos de uso corren
contra un `fakeCustomerRepository` in-memory; los tests de integración del
`CustomerRepository` de PostgreSQL se agregan junto con BE-T5.
