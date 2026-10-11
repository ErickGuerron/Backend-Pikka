# Contrato de pedidos del Sprint 1

Responsable: Anthony. Tarea S1-BE-01.

`contracts/proto/orders/v1/orders.proto` define `orders.v1.OrderService`.
El OpenAPI conserva los nombres camelCase consumidos por el frontend:
`address`, `latitude`, `longitude`, `deliveryDate`, `windowStart`, `windowEnd`,
`status`, `reviewReasons` y `createdAt`. No se modifica el cliente web existente.

| REST | gRPC | Disponibilidad |
|---|---|---|
| `POST /api/v1/orders` | `CreateOrder` | Transporte en Sprint 2 |
| `GET /api/v1/orders` | `ListOrders` | Transporte en Sprint 2 |
| `GET /api/v1/orders/{id}` | `GetOrder` | Transporte en Sprint 2 |
| Interno, para Routing | `GetPendingOrders` | Integración en Sprint 3 |
| `GET /api/v1/orders/exceptions` | `ListOrderExceptions` | Sprint posterior, contrato REST preexistente |
| `POST /api/v1/orders/{id}/review` | `ReviewOrder` | Sprint posterior, contrato REST preexistente |

`created_at` registra un instante en UTC asignado por el servidor.
`delivery_date` es una fecha civil `YYYY-MM-DD` en la zona de operación.
Las ventanas son horas locales `HH:mm` del mismo día; el fin es posterior al
inicio. Al crear, la fecha de entrega no puede preceder al día local de registro.
La ubicación exige ambas coordenadas WGS 84 finitas; la presencia explícita en
Protobuf distingue el cero geográfico de un campo omitido. La dirección recortada tiene entre
5 y 300 caracteres Unicode.

`ListOrders` admite filtros opcionales por fecha de entrega y estado; conserva
la respuesta REST como arreglo. Ordena por creación descendente e ID ascendente
en empates. `GetPendingOrders` exige fecha y selecciona solo `PENDING`, excluyendo
pedidos en revisión. Los valores de enum `UNSPECIFIED` no son estados persistibles.

Los prefijos de los enums gRPC se convierten a los valores REST existentes:
`ORDER_STATUS_PENDING` corresponde a `PENDING`. Las operaciones de revisión se
declaran para cubrir el OpenAPI existente; su implementación no forma parte de
las dos tareas de Anthony del Sprint 1. Tampoco se agrega todavía `customer_id`,
cuya relación y migración están previstas para Sprint 2.

Entrada inválida produce `INVALID_ARGUMENT`; ID ausente en la base,
`NOT_FOUND`; conflicto de revisión con el estado actual, `ABORTED` (HTTP 409).
El Gateway aplicará
el formato de error común. La autorización usa metadata propagada por el Gateway;
el contrato no publica los servicios internos al navegador.

Validación: `buf lint`, `buf breaking` contra `origin/develop`, regeneración
reproducible con las versiones de plugins fijadas en CI y `make openapi-lint`.
La implementación de dominio se revisa después de este contrato.
