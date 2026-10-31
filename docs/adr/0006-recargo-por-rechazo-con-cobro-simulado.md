# ADR 0006: Recargo por rechazo injustificado con cobro simulado

Estado: aceptada (MVP). Implementada en el backend (Order Service y Gateway); lo que falta y
lo que se apartó de la letra de este documento está en la sección 7.

## Contexto

Un pedido `NOT_DELIVERED` por rechazo del cliente sin justificación válida, con
el repartidor ya en el domicilio, genera un recargo. El negocio decidió un 15 %
del precio base del pedido no entregado (ver la decisión D6' en la sección 1),
aplicable igual en efectivo y con tarjeta.

El alcance del proyecto (enunciado) no incluye pagos, cobros ni tarjetas. Para
el MVP no se conecta una pasarela de pagos real. Tampoco se almacena ningún dato
de tarjeta.

## Decisión

### 1. Regla del recargo (se mantiene)

- **Condición**: el repartidor llegó dentro de la ventana y el rechazo del
  cliente no tiene justificación válida. Las justificaciones válidas son una
  lista cerrada del negocio.
- **Monto**: 15 % del precio base del pedido no entregado (suma de precio por cantidad de sus
  productos), redondeado a centavos. Se fija cuando ocurre la incidencia, no con el siguiente pedido:
  así el saldo es real desde el primer día y una cuenta bloqueada siempre tiene un monto que pagar
  (decisión D6'). Un pedido sin productos, anterior al catálogo, no tiene precio base: su incidencia se
  registra sin monto y no suma al saldo.
- **Una sola vez por incidencia.** No se acumula sobre el mismo pedido.
- **Quién lo aplica**: el sistema, según la regla. El repartidor solo registra el
  hecho y el motivo (ADR 0007). El operador atiende las disputas.
- **Enmienda, dirección incorrecta**: cuando el motivo es `WRONG_ADDRESS`
  (dirección mal ingresada o inválida), el recargo pasa a estado
  **`PENDING_VERIFICATION`**. No se cobra ni se notifica como definitivo hasta
  que un operador valide con el cliente, o determine por qué el repartidor no
  está en la dirección del pedido (por ejemplo, el cliente está fuera de casa y
  no hay nadie que reciba). Ningún cobro simulado se registra en este estado.
  Si el repartidor la reporta en el punto de entrega, el recargo se cobra; si
  el cliente la avisa antes de que el pedido salga, no se cobra y puede
  reprogramar la fecha. Si el repartidor ya va en camino y el cliente da un
  motivo, el pedido se reasigna con un cobro adicional por el viaje del
  repartidor, calculado por distancia con el precio de la gasolina como
  referencia y ajustable por un administrador. Ese cobro se incluye también en el
  caso de dirección errónea.
  **Resolución del operador (decisión D4):** el operador decide una de dos
  cosas, siempre con un motivo de 5 a 300 caracteres que queda auditado junto
  con su identidad y la hora. `CONFIRM`: el cliente es responsable, el recargo
  sale de verificación y sigue el flujo normal (el 15 % ya fijado, que entra al saldo en ese momento).
  `WAIVE`: el cliente no es responsable y el recargo se dispensa, sin cobro. Solo
  se resuelve una vez. Mientras espera no se cobra ni se notifica como
  definitivo. El cobro adicional por reasignación se define aparte (tarea T5).
- **Reincidencia** (plazo de reincidencia, distinto del plazo de pago de la sección 5):
  la primera no entrega causada por el cliente inicia un plazo de 2 semanas. Una
  tercera dentro de ese plazo marca al cliente como reincidente y el operador
  puede aplicar el cobro con justificación, sujeto a auditoría. Cuenta cualquier
  motivo de no entrega causado por el cliente.
  **Cómo se aplica (decisiones D7):** el plazo son 14 días contados desde la primera
  incidencia que cuenta, el día 14 incluido, y **no se desliza**: la primera incidencia
  posterior al plazo abre uno nuevo. Cuentan los recargos pendientes, aplicados o
  pagados; no cuentan los dispensados ni los que esperan verificación (un
  `WRONG_ADDRESS` cuenta desde que el operador lo confirma). La marca de reincidente
  se pone una sola vez, conserva su primera fecha y es permanente.
  **Cobro al reincidente (decisión D7b):** "el cobro" es un **recargo extra** cuyo monto
  y justificación (5 a 300 caracteres) define el operador. Se suma al saldo, queda
  auditado con su identidad y la hora, y se aplica una vez por reincidencia.

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

> **Enmienda de implementación (D6' y S10).** El monto ya existe desde la incidencia y
> el cliente lo paga desde la app cuando quiera, sin esperar a un pedido nuevo: una cuenta
> bloqueada no puede crear pedidos, así que no podría pagar "con el siguiente". El pedido
> siguiente muestra lo que se debe como línea aparte, pero no lo cobra por su cuenta.

Para que el saldo no quede indefinido:

- **Plazo** (plazo de pago, distinto del plazo de reincidencia de la sección 1): el reloj
  de 2 semanas empieza cuando el sistema confirma la penalización y notifica al
  cliente en la app. El repartidor no inicia el reloj.
  *Implementación:* el reloj empieza al confirmarse la penalización (al ocurrir, o
  cuando el operador confirma un `WRONG_ADDRESS`), y el aviso en la app (`SURCHARGE_APPLIED`) se crea en la misma
  transacción que suma el recargo al saldo. La web y la app móvil muestran el aviso.
- **Bloqueo**: si pasadas 2 semanas el saldo sigue sin pagar, la cuenta pasa a
  `BLOQUEADO_POR_SALDO`. El cliente no puede crear pedidos ni cancelarlos.
  Puede seguir entrando a la app y ver el saldo.
  *Implementación:* "pasadas" es estrictamente más de 14 días; a los 14 justos aún no.
  Un barrido periódico (al arrancar y cada `BALANCE_SWEEP_MINUTES`, 15 por defecto)
  bloquea las cuentas `ACTIVO` vencidas y nunca pisa un bloqueo por incidencias. Crear
  un pedido estando bloqueado por saldo se rechaza con 403 (enmienda al ADR 0007, sección 3:
  el bloqueo por incidencias sí va a la cola de excepciones).
- **Pago**: el cliente paga el saldo desde la app con el cobro simulado. El pago
  queda registrado como `SIMULATED_APPROVED` en `payment_attempts`.
  *Implementación:* solo se paga el saldo completo, con la tarjeta simulada (el efectivo lo
  confirma el repartidor, no la app). Un rechazo simulado también queda registrado
  (`SIMULATED_DECLINED`) y no cambia nada. Si el saldo cambia entre el cobro y el cierre,
  el cobro se devuelve y queda su reembolso registrado.
- **Desbloqueo**: al pagar el saldo completo, la cuenta vuelve a `ACTIVO` en
  ese mismo momento. No hace falta intervención del operador.
  *Implementación:* un pago nunca levanta un bloqueo por incidencias.
- **Disputa abierta**: mientras haya una disputa abierta sobre esa penalización,
  el reloj se pausa y no se bloquea la cuenta.
- **Bloqueo por incidencias**: es un mecanismo distinto, según el umbral de
  incidencias del ADR 0005 y del ADR 0007. Este bloqueo es solo por saldo.

### 6. Cumplimiento

Un recargo por incumplimiento puede considerarse penalidad. En un producto real
debe revisarse con un abogado y con la autoridad de protección al consumidor
correspondiente antes de cobrarlo. En el MVP es simulado y no hay cobro real.

### 7. Estado de la implementación

Reglas implementadas y dónde viven (Order Service, salvo que se indique):

| Regla | Dónde |
|---|---|
| Recargo del 15 % al ocurrir, una vez por pedido | `domain.Surcharge.Price`, tabla `surcharges` (`order_id` único), `RecordStopOutcome` |
| `WRONG_ADDRESS` en verificación y decisión del operador | `PENDING_VERIFICATION`, `GET /surcharges/pending-verification`, `POST /surcharges/{id}/verification` |
| Reincidencia y recargo extra | `domain.ReincidenceReached`, `customers.recidivist_at`, `POST /customers/{id}/recidivism-charge` |
| Cobro simulado y registro de intentos | `PaymentGateway`, adaptador `payments.Simulated`, tabla `payment_attempts` |
| Bloqueo por saldo vencido | `BlockOverdueBalances` (barrido), `domain.BalanceOverdue` |
| Pago del saldo y desbloqueo | `POST /customers/me/balance/payment` (Gateway), `BalanceRepository.SettleBalance` |
| El cliente ve su saldo | `GET /customers/me/balance`; `pendingSurcharge` en la respuesta de crear pedido |
| Recuperar un recargo que no se registró al cerrar la parada | `order_penalty_reasons` (motivo guardado con el cierre), `ReconcileIncidents` en el barrido |
| Aviso al cliente al confirmarse el recargo y al bloquearse la cuenta | `SURCHARGE_APPLIED` y `ACCOUNT_BLOCKED_BY_BALANCE` en `customer_notifications`; panel de avisos de la web y de la app móvil |
| Listado de reincidentes para el operador | `GET /customers/recidivists` (ADMIN y OPERATOR), solo nombre, teléfono, fecha de la marca, saldo y si el recargo extra ya se aplicó |
| Tope del recargo extra y quitar la marca | `RECIDIVISM_CHARGE_MAX` (400 con el tope si se supera); `POST /customers/{id}/recidivist/clear` solo ADMIN, auditado en `recidivist_clears` |
| Cobro adicional por reasignación | Tarifa fija que define un ADMIN (`PUT /settings/reassignment-fee`, historial en `reassignment_fees`); `POST /orders/{id}/reassignment-charge` la cobra una vez por pedido y entra al barrido y al pago |
| Pantallas del cliente (saldo, pago, etiqueta «simulado», bloqueo) | Frontend-Pikka: PR 47 (web) y PR 48 (móvil), `BalanceCard` |

Los contratos están en `contracts/openapi/openapi.yaml` (etiqueta `surcharges`) y el detalle de
tareas, decisiones y pruebas en `odd/tasks/penalty-engine.md`.

**Apartados de la letra de este ADR (decididos):** el monto se fija al ocurrir y no con el siguiente
pedido (D6'); el saldo se paga desde la app y no se cobra automáticamente con el siguiente pedido;
crear un pedido con la cuenta bloqueada por saldo se rechaza.

**Pendiente o fuera de alcance** (el detalle de cada función, su orden, las decisiones que necesita y sus criterios
de aceptación están en [`docs/planning/penalizaciones-pendientes.md`](../planning/penalizaciones-pendientes.md)):

- Pausa del reloj por disputa abierta y apelación (ADR 0007, sección 6): las disputas aún no existen. Hay una
  especificación mínima aprobada (reclamo en 2 horas, resuelve ADMIN u OPERATOR, el barrido no bloquea mientras está
  abierta) pendiente de programar; el cobro reducido, la reasignación y la pausa de la reincidencia quedan fuera.
- "Pago sin cuenta", que el ADR 0005 atribuye a este documento: descartado por producto, no existe en el MVP.
- Caducidad automática de la marca de reincidente: hoy solo un ADMIN la quita.
- Aviso al cliente del cobro por reasignación y un posible tope de la tarifa fija.
- Las incidencias de pedidos sin productos (anteriores al catálogo) se registran sin monto y no
  suman al saldo ni pueden bloquear la cuenta.

## Consecuencias

- Pagos sale del alcance real. Se reduce la superficie de seguridad y el alcance
  de PCI, que no aplica porque no hay datos de tarjeta.
- Una pasarela real puede conectarse después implementando `PaymentGateway`, sin
  cambiar las reglas de recargo.
- Pruebas obligatorias: el recargo es exactamente 15 % del precio base y se
  aplica una sola vez por incidencia; el cobro simulado nunca se presenta como
  cargo real; ningún log, respuesta ni tabla contiene datos de tarjeta.
