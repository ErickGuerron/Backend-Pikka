# ADR 0005: Registro de clientes y responsabilidad por pedido

Estado: reemplazada por el ADR 0007 (el cliente es un usuario con cuenta en la app móvil). Las reglas de cancelación y de no entregado se conservan en el ADR 0007.

## Contexto

El cliente no tiene cuenta ni inicia sesión (ADR 0004). Sin embargo, el negocio
necesita saber quién encargó cada pedido, para contactarlo, para cumplir las
reglas de cancelación y para exigir responsabilidad cuando un pedido ya
despachado no se recibe.

Por eso el cliente se registra como **entidad de negocio** vinculada a sus
pedidos, no como usuario del sistema.

## Decisión

### 1. Entidad Cliente

Vive en el esquema `orders` (Order Service) y tiene estos campos:

| Campo | Obligatorio | Notas |
|---|---|---|
| `id` | Sí | Identificador interno |
| `full_name` | Sí | Nombre con el que el negocio identifica al cliente |
| `phone` | Sí | Canal principal de contacto y envío del enlace de seguimiento |
| `email` | No | Canal alterno |
| `status` | Sí | `ACTIVO` o `BLOQUEADO` (ver sección 5) |
| `created_at` | Sí | RFC 3339, UTC |

Solo `ADMIN` y `OPERATOR` crean y consultan clientes. `DRIVER` nunca ve los
datos del cliente, ni siquiera el nombre.

### 2. Relación con el pedido

Todo pedido tiene `customer_id` obligatorio. Un pedido no puede registrarse sin
cliente. El enlace de seguimiento (ADR 0004) se envía al `phone` del cliente.

### 3. Cancelación del pedido

El operador puede cancelar un pedido con motivo registrado, solo mientras esté
en estado `PENDING` o `ROUTE_ASSIGNED`. Una vez que el repartidor marca
`PICKED_UP`, el pedido ya no se cancela: solo puede terminar como entrega o
como no entregado.

### 4. No entregado por causa del cliente

Si el repartidor llega y el pedido no se puede entregar por causa del cliente
(rechaza el pedido, no está, no atiende en la ventana), el repartidor marca el
pedido como `NOT_DELIVERED` con un motivo de catálogo: `CLIENT_REFUSED`,
`CLIENT_ABSENT` u `OUT_OF_WINDOW`. Ese evento:

- queda en el historial de estados con fecha, hora, actor y motivo;
- suma una incidencia al registro del cliente;
- no se puede revertir a `PENDING` sin una acción manual del operador, que se
  registra como nuevo intento.

Estado nuevo: `NOT_DELIVERED`. Se agrega a la lista de estados de
`BASE_TECNICA.md`, sección 3.5.

### 5. Bloqueo por incidencias

Un cliente con incidencias acumuladas pasa a `BLOQUEADO` y el operador no puede
registrarle pedidos nuevos hasta revisarlo. El umbral (por ejemplo, 3
incidencias en 90 días) es configurable y se define con el negocio.

### 6. Consecuencias económicas

Un pedido `NOT_DELIVERED` por rechazo sin justificación válida, con el repartidor
ya en el domicilio, genera un recargo del 15 % sobre el precio base del
siguiente pedido del cliente, con cobro en efectivo o con tarjeta tokenizada.
Las reglas completas, incluido el pago sin cuenta, están en el ADR 0006.

### 7. Auditoría

Toda transición de estado registra actor, fecha y hora, y motivo cuando aplica.
El historial no se puede editar, solo se agrega.

### 8. Privacidad

Los datos del cliente son mínimos (sección 1). El enlace de seguimiento no
muestra nombre ni teléfono del cliente. El acceso a estos datos queda limitado
a los roles `ADMIN` y `OPERATOR`.

## Consecuencias

- Order Service agrega las tablas `customers` e `incidents`, y la columna
  `customer_id` en `orders`.
- Order Service expone CRUD de clientes solo para `ADMIN` y `OPERATOR`.
- Routing Service y Driver Service no reciben datos del cliente, solo la
  dirección de la parada, que es necesaria para la ruta.
- Pruebas obligatorias: no se puede cancelar un pedido en `PICKED_UP`; un cliente
  `BLOQUEADO` no recibe pedidos nuevos; un `DRIVER` no obtiene datos del cliente;
  cada transición deja registro de auditoría.
