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


## Recargos, saldo y bloqueo (ADR 0006)

Operaciones de `orders.v1.OrderService` agregadas para el motor de penalizaciones. Los roles los
verifica el Gateway (`StaffOnly`, `CustomerOnly`) y otra vez Order Service. La identidad auditada sale
siempre del JWT, nunca del cuerpo. El dinero viaja como texto decimal con dos decimales; un número JSON se
rechaza con 400.

| REST | gRPC | Rol | Notas |
|---|---|---|---|
| `GET /api/v1/surcharges/pending-verification` | `ListSurchargesPendingVerification` | ADMIN, OPERATOR | Cola de recargos `WRONG_ADDRESS`, del más antiguo al más reciente |
| `POST /api/v1/surcharges/{id}/verification` | `ResolveSurchargeVerification` | ADMIN, OPERATOR | `CONFIRM` o `WAIVE` con motivo de 5 a 300 caracteres; 409 si ya se resolvió |
| `POST /api/v1/customers/{id}/recidivism-charge` | `ApplyRecidivismCharge` | ADMIN, OPERATOR | 201; 422 si no es reincidente; 409 si ya se cobró esa reincidencia |
| `POST /api/v1/customers/me/balance/payment` | `PayMyBalance` | CUSTOMER | Sin cuerpo: paga el saldo completo con la tarjeta simulada; siempre `simulated: true` |
| `GET /api/v1/customers/me/balance` | `GetMyBalance` | CUSTOMER | Saldo, estado de la cuenta y cada línea con su fecha límite; una cuenta bloqueada puede verlo |

Cambios a operaciones existentes, todos aditivos salvo los dos 403:

- `RecordRouteStopOutcome` recibe `reason` (opcional): un motivo del cliente registra la incidencia y fija el
  recargo del 15 % sobre el precio base del pedido no entregado.
- `CreateOrderResponse` incluye `pending_surcharge`, aparte de `total`.
- `Customer` incluye `recidivist_at` (vacío si no es reincidente).
- Una cuenta `BLOQUEADO_POR_SALDO` recibe `PERMISSION_DENIED` (403) al crear o cancelar pedidos.

Códigos: sin saldo que pagar, `FAILED_PRECONDITION` (422); saldo que cambió mientras se pagaba, `ABORTED` (409,
el cobro se devuelve); recargo o cliente inexistente, `NOT_FOUND`. Un rechazo de la tarjeta simulada **no** es un
error: responde 200 con `SIMULATED_DECLINED`.
