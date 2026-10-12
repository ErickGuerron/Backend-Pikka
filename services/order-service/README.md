# order-service

Pendiente. Se implementa según el plan de la sección 46 de la base técnica.

## Estado (BE-T4a — esqueleto)

Este commit agrega únicamente la estructura mínima del servicio: arranque HTTP,
`/health` y `/ready`, conexión a PostgreSQL sobre el esquema `orders`. Sin
dominio, sin gRPC, sin handlers de negocio. **BE-T4b** introduce el dominio
Customer y la migración real (`customers`) sobre el mismo esquema.

## Modelo

- Esquema `orders` (creado por `infrastructure/postgres/init/01-schemas-and-users.sh`).
- Usuario `order_service_user`, dueño del esquema.
- En BE-T4b se agrega la tabla `customers`.

## Pruebas

```bash
go test ./...
```

Sin pruebas de integración todavía; se agregan junto con el dominio.
