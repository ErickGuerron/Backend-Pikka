# ADR 0006: Recargo por rechazo injustificado con cobro simulado

Estado: aceptada (MVP)

## Contexto

Un pedido `NOT_DELIVERED` por rechazo del cliente sin justificación válida, con
el repartidor ya en el domicilio, genera un recargo. El negocio decidió un 15 %
del precio base del siguiente pedido, aplicable igual en efectivo y con tarjeta.

El alcance del proyecto (enunciado) no incluye pagos, cobros ni tarjetas. Para
el MVP no se conecta una pasarela de pagos real. Tampoco se almacena ningún dato
de tarjeta.

## Decisión

### 1. Regla del recargo (se mantiene)

- **Condición**: el repartidor llegó dentro de la ventana y el rechazo del
  cliente no tiene justificación válida. Las justificaciones válidas son una
  lista cerrada del negocio.
- **Monto**: 15 % del precio base del siguiente pedido del cliente.
- **Una sola vez por incidencia.** No se acumula sobre el mismo pedido.
- **Quién lo aplica**: el sistema, según la regla. El repartidor solo registra el
  hecho y el motivo (ADR 0007). El operador atiende las disputas.
- **Enmienda, dirección incorrecta**: cuando el motivo es `WRONG_ADDRESS`
  (dirección mal ingresada o inválida), el recargo pasa a estado
  **`PENDING_VERIFICATION`**. No se cobra ni se notifica como definitivo hasta
  que un operador lo confirme. Si el operador lo confirma, se aplica la regla
  normal; si lo descarta, el recargo se anula. Ningún cobro simulado se registra
  en este estado.

### 2. Cobro simulado

- Order Service define una interfaz `PaymentGateway` con operaciones de cobro y
  reembolso.
- El MVP usa un adaptador de simulación que responde aprobado o rechazado según
  reglas de prueba. No hay llamadas a red ni a un proveedor externo.
- Cada intento de cobro se registra en la tabla `payment_attempts` con monto,
  estado (`SIMULATED_APPROVED` o `SIMULATED_DECLINED`), método (`CASH` o
  `CARD`), referencia interna y fecha.
- El cobro en efectivo se registra cuando el repartidor confirma que recibió el
  dinero. El cobro con tarjeta se registra con el adaptador de simulación.

### 3. Sin datos de tarjeta

- El sistema **no pide, no almacena y no registra** número de tarjeta,
  vencimiento ni CVV, ni siquiera en simulación.
- Al cliente solo se le puede mostrar la marca y los últimos 4 dígitos, y solo
  si la simulación los devuelve. En el MVP ni siquiera eso es necesario.
- El método de pago es una preferencia del cliente (`CASH` o `CARD`), sin datos
  sensibles.

### 4. Saldo y visibilidad

- `customers.pending_surcharge` acumula el recargo pendiente.
- El recargo aparece como línea separada en el siguiente pedido y en la app del
  cliente, con su motivo, antes de que el pedido se confirme.
- El cobro se marca como simulado en la interfaz para que nadie crea que hubo un
  cargo real.

### 5. Saldo pendiente y bloqueo por falta de pago

El recargo queda como saldo pendiente del cliente. Se cobra con su próximo
pedido, sin importar cuándo ocurra (opción A).

Para que el saldo no quede indefinido:

- **Plazo**: el reloj de 2 semanas empieza cuando el sistema confirma la
  penalización y notifica al cliente en la app. El repartidor no inicia el
  reloj.
- **Bloqueo**: si pasadas 2 semanas el saldo sigue sin pagar, la cuenta pasa a
  `BLOQUEADO_POR_SALDO`. El cliente no puede crear pedidos ni cancelarlos.
  Puede seguir entrando a la app y ver el saldo.
- **Pago**: el cliente paga el saldo desde la app con el cobro simulado. El pago
  queda registrado como `SIMULATED_APPROVED` en `payment_attempts`.
- **Desbloqueo**: al pagar el saldo completo, la cuenta vuelve a `ACTIVO` en
  ese mismo momento. No hace falta intervención del operador.
- **Disputa abierta**: mientras haya una disputa abierta sobre esa penalización,
  el reloj se pausa y no se bloquea la cuenta.
- **Bloqueo por incidencias**: es un mecanismo distinto, según el umbral de
  incidencias del ADR 0005 y del ADR 0007. Este bloqueo es solo por saldo.

### 6. Cumplimiento

Un recargo por incumplimiento puede considerarse penalidad. En un producto real
debe revisarse con un abogado y con la autoridad de protección al consumidor
correspondiente antes de cobrarlo. En el MVP es simulado y no hay cobro real.

## Consecuencias

- Pagos sale del alcance real. Se reduce la superficie de seguridad y el alcance
  de PCI, que no aplica porque no hay datos de tarjeta.
- Una pasarela real puede conectarse después implementando `PaymentGateway`, sin
  cambiar las reglas de recargo.
- Pruebas obligatorias: el recargo es exactamente 15 % del precio base y se
  aplica una sola vez por incidencia; el cobro simulado nunca se presenta como
  cargo real; ningún log, respuesta ni tabla contiene datos de tarjeta.
