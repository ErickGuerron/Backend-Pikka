# ADR 0007: Alcance del MVP con app móvil única y validación automática de pedidos

Estado: aceptada. Enmendada en lo referente a productos por el ADR 0009 (catálogo con stock simple).

Reemplaza a los ADR 0004 (seguimiento por enlace protegido) y 0005 (cliente
como entidad sin cuenta). Mantiene el ADR 0006 con las precisiones de cobro
simulado.

## Contexto

Durante el diseño se evaluaron varias formas de que el cliente interactúe con el
sistema. Se decidió un modelo con clientes registrados en la app móvil, con el
operador como responsable de las excepciones y no de la captura rutinaria. El
enunciado no pide pagos reales, así que el MVP los simula.

## Decisión

### 1. Aplicación móvil única

Una sola app móvil atiende a dos roles:

- **CUSTOMER**: se registra, crea pedidos, sigue su estado y su ruta, y puede
  presentar una disputa sobre un rechazo.
- **DRIVER**: consulta su ruta asignada, marca cada parada y registra el
  resultado de cada entrega.

La web es para `ADMIN` y `OPERATOR`. Un `DRIVER` no usa la web.

### 2. Cuentas

- Los clientes se registran con correo y contraseña. El registro es autónomo
  desde la app.
- Los repartidores los da de alta un `ADMIN`, como hasta ahora.
- Auth Service agrega el rol `CUSTOMER`. Los roles son `ADMIN`, `OPERATOR`,
  `DRIVER` y `CUSTOMER`.

### 3. Validación automática de pedidos

Al crear un pedido, el sistema valida solo:

- la dirección cae en una zona atendida (`ClassifyPoint`);
- la fecha y la ventana son válidas;
- la capacidad de la fecha lo permite (`MAX_ORDERS_PER_ROUTE`);
- el cliente no está bloqueado.

Si todo cumple, el pedido queda confirmado y es planificable. Si algo no cumple,
el pedido queda en una **cola de excepciones** para el operador. El operador
confirma, corrige o rechaza, con motivo. El cliente recibe la decisión en la app.

El operador deja de capturar pedidos rutinarios. Su trabajo es la excepción.

### 4. Registro de la entrega

El repartidor registra el resultado de cada parada:

- `DELIVERED`, o
- `NOT_DELIVERED` con motivo de una lista cerrada: `CLIENT_REFUSED`,
  `CLIENT_ABSENT`, `OUT_OF_WINDOW`, o un motivo imputable al negocio (producto
  dañado, dirección errónea, pedido duplicado).

Cada registro incluye hora y ubicación GPS. Una foto es opcional.

### 5. Penalización aplicada por el sistema

El sistema decide si corresponde recargo, no el repartidor:

- Verifica que la llegada cuenta como válida: GPS dentro de un radio de la
  dirección y dentro de la ventana.
- Si el motivo es imputable al cliente, aplica el recargo del ADR 0006 y
  notifica al cliente en la app, con el motivo.
- Si el motivo es imputable al negocio, no aplica recargo.

### 6. Disputas

El cliente puede presentar una disputa desde la app dentro de un plazo (por
ejemplo 2 horas desde la notificación). La disputa pasa a la cola del operador.
Mientras está abierta, el recargo queda en suspenso.

### 7. Pagos

Sin pasarela real. Cobro simulado según el ADR 0006. No se almacenan datos de
tarjeta.

### 8. Cancelación

- El cliente puede cancelar su pedido desde la app solo en `PENDING` o
  `ROUTE_ASSIGNED`.
- El operador puede cancelar con motivo registrado, en esos mismos estados.
- Desde `PICKED_UP` el pedido no se cancela. Solo puede terminar como entregado
  o como no entregado.

### 9. Fuera del MVP

- Pasarela de pagos real y tokenización de tarjetas.
- Reembolsos.
- Seguimiento por enlace sin cuenta.
- Revisión automática avanzada de disputas.
- Cobro del saldo pendiente sin pedido siguiente (decisión pendiente del
  ADR 0006).

## Consecuencias

- **Auth Service**: agrega `CUSTOMER`, registro autónomo de clientes y
  validación de rol en las rutas nuevas.
- **Order Service**: validación automática, cola de excepciones, relación con el
  cliente y el ledger de cobros simulados.
- **Routing Service**: planifica solo pedidos confirmados y expone el estado de
  la ruta al cliente con los datos mínimos.
- **Driver Service**: el repartidor registra el resultado de la parada. Se valida
  la prueba de llegada.
- **Mobile**: la app se diseña con dos flujos de rol. Esto cambia la navegación de
  los wireframes de Sprint 1 y el plan de Mabe.
- **Pruebas obligatorias**:
  - un cliente no ve pedidos ajenos (BOLA);
  - un repartidor no ve datos personales del cliente;
  - un rechazo sin llegada verificada no genera recargo;
  - el recargo es exactamente 15 % y se aplica una vez;
  - una disputa suspende el recargo;
  - ningún log ni respuesta contiene datos de tarjeta.
- **Seguridad**: ZAP autenticado con sesión de cliente y sesión de repartidor.
