# Pedido al backend: signup CUSTOMER + endpoints de pedido del cliente

**De:** Mabe (frontend, apps/mobile).
**Para:** Anthony (backend) con apoyo de Erick (full stack).
**Contexto:** Feature ODD `frontend-pikka/odd/mvp-customer-app.md`. ADR 0006, 0007 y 0008 ya están mergeados en `develop`.

## Resumen

El frontend móvil necesita seis endpoints nuevos (o ampliados) para implementar el flujo del cliente del MVP. La implementación de estos endpoints es **tarea del equipo backend**, no de Mabe. Este doc deja el contrato esperado y la matriz de pruebas obligatorias para que Anthony/Erick los entreguen en su próximo sprint.

## 1. Ampliación de auth.proto

Agregar al enum `Role` en `contracts/proto/auth/v1/auth.proto`:

```proto
enum Role {
  ROLE_UNSPECIFIED = 0;
  ROLE_ADMIN = 1;
  ROLE_OPERATOR = 2;
  ROLE_DRIVER = 3;
  ROLE_CUSTOMER = 4;  // NUEVO
}
```

Actualizar `buf breaking` (cambio compatible: agregar valor al final).

## 2. Endpoint público de signup de cliente

**Ruta Gateway:** `POST /api/v1/auth/signup`
**Auth:** ninguna (público, como `auth/login`).
**Rate limit:** 5 requests / IP / minuto (mismo criterio que el resto del Gateway).

**Request:**

```json
{
  "email": "cliente@example.com",
  "password": "minimo8caracteres",
  "fullName": "Nombre Apellido"
}
```

**Response 201:**

```json
{
  "accessToken": "eyJ...",
  "tokenType": "Bearer",
  "expiresInSeconds": 3600,
  "user": {
    "id": "uuid",
    "email": "cliente@example.com",
    "fullName": "Nombre Apellido",
    "role": "CUSTOMER",
    "active": true,
    "createdAt": "2026-10-11T15:00:00Z"
  }
}
```

**Errores:**

- `409 Conflict` si el email ya existe.
- `400 Bad Request` si email inválido o contraseña < 8 caracteres.
- `429 Too Many Requests` si supera el rate limit.

**Implementación backend:** `auth-service` agrega `SignupCustomer` al `AuthService` gRPC. Validación de email único antes de crear. Contraseña hasheada con bcrypt o argon2 (mismo criterio que `CreateUser` actual). El Gateway expone el endpoint HTTP sin exigir JWT, pero sí exige rate limit.

**Pruebas obligatorias:**

- Email duplicado → 409.
- Email inválido → 400.
- Contraseña < 8 caracteres → 400.
- Email nuevo válido → 201 con token JWT que contiene rol `CUSTOMER`.
- El token emitido sirve para `GET /api/v1/auth/me` y devuelve `role: "CUSTOMER"`.

## 3. Endpoints de pedidos filtrados por cliente

ADR 0008 §3 dice: "cada endpoint filtra por alcance del rol antes de consultar". Para CUSTOMER, esto significa que **nunca** devuelve pedidos ajenos.

### 3.1 `GET /api/v1/orders` (CUSTOMER scope)

**Auth:** JWT con rol CUSTOMER.
**Comportamiento:** devuelve solo los pedidos cuyo `customer_id` coincide con el `sub` del JWT. **Ignora** cualquier query param `customerId` enviado por el cliente.
**Errores:** `403` si el rol no es CUSTOMER, OPERATOR o ADMIN.

**Response 200:**

```json
{
  "items": [
    {
      "id": "uuid",
      "status": "PENDING",
      "deliveryDate": "2026-10-15",
      "deliveryWindow": "09:00-12:00",
      "address": "Av. Siempre Viva 742",
      "stopPosition": 3,
      "totalStops": 8,
      "createdAt": "2026-10-11T15:00:00Z"
    }
  ],
  "total": 1
}
```

### 3.2 `POST /api/v1/orders` (CUSTOMER scope)

**Auth:** JWT con rol CUSTOMER.
**Comportamiento:** crea pedido con validación automática (ADR 0007 §3). Si pasa: estado `PENDING`. Si no: estado `PENDING_REVIEW` y queda en la cola de excepciones del operador.

**Request:**

```json
{
  "address": "Av. Siempre Viva 742",
  "deliveryDate": "2026-10-15",
  "deliveryWindow": "09:00-12:00",
  "notes": "Tocar timbre 2B"
}
```

**Errores:** `403` si el rol no es CUSTOMER, OPERATOR o ADMIN. `400` si `customer` está en estado `BLOQUEADO_POR_SALDO` (ADR 0006 §5).

### 3.3 `GET /api/v1/orders/{id}` (CUSTOMER scope)

**Auth:** JWT con rol CUSTOMER.
**Comportamiento:** devuelve el pedido solo si pertenece al cliente autenticado. Si pertenece a otro cliente: **`404` (no `403`)**, para no revelar que existe (ADR 0008 §3).

**Response 200:** pedido completo con `stopPosition`, `totalStops`, `eta`, `lastPosition` (si está disponible) y `status`.

### 3.4 `PATCH /api/v1/orders/{id}/cancel` (CUSTOMER scope)

**Auth:** JWT con rol CUSTOMER, dueño del pedido.
**Comportamiento:** cancela solo si el estado es `PENDING` o `ROUTE_ASSIGNED` (ADR 0007 §8). Desde `PICKED_UP` no se cancela.

**Errores:**

- `404` si el pedido no es del cliente.
- `409 Conflict` si el estado no permite cancelación, con mensaje `"El pedido ya fue recogido y no puede cancelarse"`.

### 3.5 `POST /api/v1/orders/{id}/dispute` (CUSTOMER scope)

**Auth:** JWT con rol CUSTOMER, dueño del pedido.
**Comportamiento:** abre una disputa sobre un pedido en estado `NOT_DELIVERED`. Solo dentro del plazo (2 horas desde la notificación, configurable). Mientras esté abierta, el recargo queda en suspenso (ADR 0007 §6).

**Request:**

```json
{
  "reason": "Estaba en casa, el repartidor nunca tocó el timbre"
}
```

**Errores:**

- `404` si el pedido no es del cliente.
- `409 Conflict` si el pedido no está en `NOT_DELIVERED` o ya pasó el plazo.
- `409 Conflict` si ya hay una disputa abierta para ese pedido.

### 3.6 `GET /api/v1/orders/{id}/position` (CUSTOMER scope)

**Auth:** JWT con rol CUSTOMER, dueño del pedido.
**Comportamiento:** devuelve la última ubicación del repartidor asignado al pedido. Si no hay repartidor asignado o el pedido está entregado: `204 No Content`.

**Response 200:**

```json
{
  "latitude": -0.1807,
  "longitude": -78.4849,
  "recordedAt": "2026-10-15T10:23:00Z"
}
```

**Dependencia:** requiere Fase 6 (Tracking Service) en producción. Mientras esa fase no exista, el endpoint devuelve `204` siempre y el frontend muestra el mapa sin pin del repartidor.

## Matriz de pruebas obligatorias (BOLA)

| Caso | Resultado esperado |
|---|---|
| CUSTOMER A pide `GET /orders/{id-de-B}` | `404` (no `403`) |
| CUSTOMER A pide `PATCH /orders/{id-de-B}/cancel` | `404` |
| CUSTOMER A pide `POST /orders/{id-de-B}/dispute` | `404` |
| CUSTOMER A pide `GET /orders/{id-de-A}` | `200` con su pedido |
| DRIVER pide `GET /orders/{id-de-A}` (no es su ruta) | `403` o `404` según matriz ADR 0008 §2 |
| OPERATOR pide `GET /orders/{id-de-A}` | `200` (lectura operativa permitida) |
| ADMIN pide `GET /orders/{id-de-A}` | `200` |

## Plan de entrega sugerido

| Sprint | Tarea backend | Owner |
|---|---|---|
| Próximo (Sprint 2/3) | Auth: `ROLE_CUSTOMER` + `SignupCustomer` + rate limit | Anthony |
| Próximo | Orders: filtrado CUSTOMER en `GET /orders`, `POST /orders`, `GET /orders/{id}`, `PATCH /orders/{id}/cancel` | Anthony |
| Próximo | Orders: `POST /orders/{id}/dispute` + estado `BLOQUEADO_POR_SALDO` | Anthony |
| Fase 6 | `GET /orders/{id}/position` con Tracking Service | Anthony/Erick |

Mientras estos endpoints no estén, Mabe trabaja contra mocks (MSW o axios-mock-adapter) en `apps/mobile`. El contrato de este doc es la fuente de verdad.