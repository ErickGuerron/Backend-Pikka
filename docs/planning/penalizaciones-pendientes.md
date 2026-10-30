# Penalizaciones: funciones pendientes

Estado al 2026-10-30. Complementa la sección 7 del
[ADR 0006](../adr/0006-recargo-por-rechazo-con-cobro-simulado.md) y el plan
[`odd/tasks/penalty-engine.md`](../../odd/tasks/penalty-engine.md): lo que ya existe (recargo del 15 % al ocurrir,
verificación de `WRONG_ADDRESS`, reincidencia y recargo extra, bloqueo por saldo, pago simulado y vista del saldo)
no se repite aquí. Esto es lo que **falta**, por qué falta, qué hay que decidir y cómo se probará.

## Orden de ejecución

Primero lo que no necesita ninguna decisión de producto, ordenado por el daño que evita; después lo que necesita una
decisión pequeña; al final lo que necesita una especificación.

| # | Función | Necesita decisión | Dónde | Riesgo (ODD) |
|---|---|---|---|---|
| F1 | Reconciliación de incidencias perdidas | No | Order Service | item 1 (tabla nueva) y 4 (barrido) |
| F2 | Aviso en la app al confirmarse la penalización | No (el texto es de producto) | Order Service, Gateway y apps | item 3 (tipo de aviso nuevo) |
| F3 | Cancelar bloqueado en la interfaz | No | Web y móvil | ninguno |
| F4 | Listado de reincidentes para el operador | Sí, pequeña (D-F4) | Order Service y Gateway | item 3 (ruta nueva) |
| F5 | Tope del recargo extra y quitar la marca de reincidente | Sí (D-F5a, D-F5b) | Order Service y Gateway | item 1 y 3 |
| F6 | Cobro adicional por reasignación (T5) | Sí (D3) | Order Service y Gateway | item 1 y 3 |
| F7 | «Pago sin cuenta» (ADR 0005) | Sí, de producto (D2) | por definir | por definir |
| F8 | Disputas, pausa del reloj y apelación | Sí, requiere especificación | todo | item 1, 3 y 4 |

**Hechas:** F1 y F2 (web; falta la app móvil).

Cada función se entrega como su propio PR, con pruebas primero y la documentación al día. Las decisiones propuestas
(«Propuesta») son un punto de partida, no están aprobadas hasta que producto las confirme.

---

## F1. Reconciliación de incidencias perdidas

**Estado:** implementada (rama `feat/penalty-f1-reconcile`). **Decisión requerida:** ninguna.

**Qué es.** Antes de F1 el recargo se registra *después* de cerrar la parada y, si falla, solo queda un log de error
(`recordSurchargeNonFatal`). El repartidor no puede reintentar una parada ya cerrada, así que esa penalidad se pierde
en silencio. Es dinero que no se cobra y un cliente que no suma reincidencia.

**Por qué no basta con mirar los pedidos.** Order Service sabe que el pedido quedó `NOT_DELIVERED`, pero **no guarda
el motivo**: ese dato vive solo en Driver Service. Sin el motivo no se puede saber, a posteriori, si correspondía recargo.

**Diseño propuesto.**
1. Guardar el motivo en la misma transacción que cierra la parada: tabla `order_penalty_reasons`
   (`order_id` PK, `reason`, `recorded_at`), escrita por `AdvanceRouteStop` solo cuando el motivo genera incidencia
   (`CLIENT_*`, `OUT_OF_WINDOW` o `WRONG_ADDRESS`). Los pedidos cerrados antes de esta función no tienen fila y no se
   reconcilian (no hay de dónde sacar su motivo).
2. Caso de uso `ReconcileIncidents`: lista los motivos sin fila en `surcharges` (`LEFT JOIN`), y por cada uno repite
   exactamente lo que hace el cierre de parada (fijar el 15 % con el precio base, registrar la incidencia, evaluar la
   reincidencia). Es seguro repetirlo: `surcharges.order_id` es único.
3. La incidencia reconciliada conserva la **fecha original** del cierre (`recorded_at`), no la del barrido, para que el
   plazo de pago de 14 días no se alargue por la demora.
4. Corre en el mismo barrido periódico del bloqueo (`BALANCE_SWEEP_MINUTES`), **antes** de calcular los vencidos.
5. Se comparte el código con el cierre de parada (un solo lugar que registra la incidencia).

**Criterios de aceptación.**
- Si el registro falla al cerrar la parada, el siguiente barrido crea la incidencia, con el monto correcto y la fecha del cierre.
- Un pedido cuyo motivo es del negocio (`PRODUCT_DAMAGED`, `DUPLICATE_ORDER`) no genera nada.
- Repetir el barrido no duplica incidencias ni suma dos veces al saldo.
- Un pedido anterior a la función (sin fila en `order_penalty_reasons`) no se toca.
- Un fallo del barrido se registra y el siguiente pase reintenta.

**Pruebas.** Unitarias del caso de uso (con repositorio simulado) e integración contra PostgreSQL: cierre con fallo
simulado, reconciliación, idempotencia y migración subir/bajar.

**Qué se hizo.** Migración 000020 (`order_penalty_reasons`); `Order.PenaltyReason` (campo que solo viaja en la escritura
del cierre); `AdvanceRouteStop` guarda el motivo en la misma transacción; `IncidentRecorder` (el único lugar que fija
el 15 %, registra la incidencia y evalúa la reincidencia, usado por el cierre y por el barrido); `ReconcileIncidents`
(lotes de 200, del más antiguo al más reciente) y su llamada al inicio de cada pase del barrido, antes de calcular los vencidos.

**Límites conocidos.**
- Los pedidos cerrados **antes** de esta función no tienen motivo guardado y no se reconcilian.
- Un lote se toma del más antiguo: si hubiera 200 incidencias que fallan siempre, las posteriores esperarían. No se
  espera con el volumen actual; el log de error de cada una permite detectarlo.
- Una incidencia que no se pudo registrar sigue sin contar para la reincidencia ni para el saldo hasta el siguiente
  pase (como máximo `BALANCE_SWEEP_MINUTES`, 15 por defecto).

---

## F2. Aviso en la app al confirmarse la penalización

**Estado:** backend y web implementados (rama `feat/penalty-f2-notice`); **la app móvil queda pendiente (F2b)**. **Decisión requerida:** solo el texto del aviso (propuesto abajo).

**Qué es.** El ADR 0006 dice que el reloj de 2 semanas empieza «cuando el sistema confirma la penalización **y notifica
al cliente en la app**». Hoy el reloj empieza al confirmarse, pero no existe el aviso: el cliente se entera de que debe
dinero solo si abre la tarjeta de saldo.

**Diseño propuesto.**
- Dos tipos nuevos en `customer_notifications` (migración que amplía el `CHECK` de tipos):
  `SURCHARGE_APPLIED` («Se aplicó un recargo de $X por tu pedido no entregado. Pagar antes del DD/MM/AAAA.») y
  `ACCOUNT_BLOCKED_BY_BALANCE` («Tu cuenta quedó bloqueada por saldo pendiente.»).
- `SURCHARGE_APPLIED` se crea **en la misma transacción** que suma el recargo al saldo (incidencia nueva en `PENDING`,
  o confirmación del operador de un `WRONG_ADDRESS`). **No** se crea para `PENDING_VERIFICATION` ni `WAIVED`: el ADR
  dice que no se notifica como definitivo hasta que el operador lo valide.
- `ACCOUNT_BLOCKED_BY_BALANCE` lo crea el barrido al bloquear.
- Las apps los muestran en el panel de avisos que ya existe (`GET /notifications/mine`), con el texto traducido del tipo.

**Qué se hizo.** Migración 000021 (tipos nuevos y `order_id` nulo solo para el aviso de bloqueo, con una restricción que
exige pedido en todos los demás); `NewSurchargeAppliedNotification` y `NewAccountBlockedNotification`; el aviso del
recargo se inserta en la misma transacción que lo suma al saldo (incidencia nueva o confirmación del operador) y el de
bloqueo en la misma que cambia el estado de la cuenta. El detalle de `SURCHARGE_APPLIED` es `MOTIVO,MONTO,FECHA_LIMITE` sin
idioma: cada app lo traduce. La web lo muestra en su panel de avisos.

**Límites conocidos.**
- **La app móvil no tiene panel de avisos** (tampoco para los avisos de revisión de pedidos que ya existían): hoy el cliente
  móvil se entera de lo que debe por la tarjeta de saldo. Mostrar los avisos allí es una función aparte (F2b).
- Una incidencia recuperada por la reconciliación (F1) también avisa, pero con la fecha original: el aviso puede llegar
  tarde respecto del plazo.
- Los recargos creados antes de F2 no tienen aviso.
- Un cliente con la versión anterior de la web no entiende los tipos nuevos y no muestra su panel de avisos; conviene
  desplegar la web antes o junto con el backend.

**Criterios de aceptación.**
- Una incidencia nueva con monto deja un aviso con el monto y la fecha límite; una sin monto, `PENDING_VERIFICATION` o
  dispensada, no.
- Confirmar un `WRONG_ADDRESS` deja el aviso; dispensarlo, no.
- El bloqueo deja su aviso una sola vez por bloqueo.
- El aviso no contiene datos de tarjeta ni la justificación interna del operador.

---

## F3. Cancelar bloqueado en la interfaz

**Estado:** no implementada. **Decisión requerida:** ninguna.

**Qué es.** El servidor ya rechaza con 403 cancelar un pedido a una cuenta `BLOQUEADO_POR_SALDO`, pero la interfaz sigue
mostrando el botón y el cliente ve un error genérico. Hoy solo se deshabilita «Hacer nuevo pedido».

**Diseño propuesto.** En web y móvil, con la cuenta bloqueada: ocultar o deshabilitar «Cancelar pedido» con el mismo
mensaje de la tarjeta de saldo («pagar el saldo para volver a cancelar pedidos»), y, si aun así llega un 403, mostrar
ese mensaje en vez del genérico.

**Criterios de aceptación.** Con `blocked: true` no se puede iniciar la cancelación; un 403 de cancelar muestra el
mensaje del saldo; con la cuenta activa nada cambia.

---

## F4. Listado de reincidentes para el operador

**Estado:** no implementada. **Decisión pequeña pendiente (D-F4).**

**Qué es.** El operador puede aplicar el recargo extra (`POST /customers/{id}/recidivism-charge`) pero no tiene cómo
encontrar al cliente: `GET /customers` es solo ADMIN (ADR 0008).

**Propuesta (D-F4).** No abrir `GET /customers`. Crear `GET /api/v1/customers/recidivists` para ADMIN y OPERATOR que
devuelva solo lo mínimo para decidir: `id`, `fullName`, `recidivistAt`, `pendingSurcharge` y si ya tuvo recargo extra.
Sin correo ni teléfono. **Confirmar:** qué campos puede ver el operador (el ADR 0008 no lo dice).

**Criterios de aceptación.** El operador lista solo reincidentes; el cliente y el repartidor reciben 403; la respuesta no
trae datos de contacto; los más antiguos primero.

---

## F5. Tope del recargo extra y quitar la marca de reincidente

**Estado:** no implementada. **Decisiones pendientes (D-F5a, D-F5b).**

**Qué es.** (a) El recargo extra solo está limitado por la columna `NUMERIC(12,2)`: un error de tecleo con ceros de más
pasaría. (b) La marca de reincidente es permanente, así que hoy cada cliente puede recibir **un solo** recargo extra en
su vida.

**Propuesta (D-F5a).** Tope configurable por variable de entorno (`RECIDIVISM_CHARGE_MAX`); si se supera responde 400
con el tope en el mensaje. **El valor lo define el negocio**: no se asume uno.

**Propuesta (D-F5b).** `POST /customers/{id}/recidivist/clear`, **solo ADMIN**, con motivo auditado (5 a 300
caracteres), que limpia `recidivist_at` y deja el registro del cambio. Tras limpiarla, una nueva reincidencia vuelve a
marcar al cliente y permite otro recargo extra. **Confirmar:** quién puede quitarla y si hay un plazo automático
(por ejemplo, caducar a los 6 meses).

**Criterios de aceptación.** Un monto sobre el tope se rechaza sin tocar el saldo; limpiar la marca exige ADMIN y motivo,
queda auditado y no borra los recargos extra ya cobrados.

---

## F6. Cobro adicional por reasignación (T5)

**Estado:** no implementada. **Decisión requerida: D3.**

**Qué es.** El ADR 0006 (dirección incorrecta) dice que si el repartidor ya va en camino y el cliente da un motivo, el
pedido se reasigna con un cobro adicional «calculado por distancia con el precio de la gasolina como referencia y
ajustable por un administrador».

**Faltan las decisiones D3:**
1. Fórmula: ¿distancia recorrida × consumo × precio de la gasolina? ¿Con qué consumo y qué distancia (la ruta real o la
   línea recta al domicilio)?
2. Precio de la gasolina: ¿lo escribe un administrador a mano, o se toma de una fuente?
3. «Ajustable por un administrador»: ¿un factor, un monto fijo, un porcentaje? ¿Quién lo cambia y queda auditado?
4. ¿Se suma al saldo igual que el recargo, con el mismo plazo de 14 días?

**Diseño provisional (sujeto a D3).** Tabla `pricing_parameters` (precio de la gasolina, factor) editable por ADMIN con
auditoría; el cobro se registra como una línea más del saldo (`REASSIGNMENT`), por lo que entra en el barrido, el pago y
la vista del saldo sin cambios.

---

## F7. «Pago sin cuenta» (ADR 0005)

**Estado:** sin definir. **Decisión de producto requerida: D2.**

El ADR 0005 dice que las reglas del «pago sin cuenta» están en el ADR 0006, pero el 0006 no lo define. Hay que decidir,
al menos: qué es (¿un cliente que pide sin registrarse?), cómo se le cobra un recargo si no tiene cuenta que bloquear, y
qué identifica a esa persona. Mientras no exista esa definición no hay nada que implementar; este documento no la inventa.

---

## F8. Disputas, pausa del reloj y apelación

**Estado:** no implementada; **las disputas no existen en el sistema.** **Requiere especificación.**

**Qué dice el ADR.** ADR 0006 §5: mientras haya una disputa abierta sobre la penalización, el reloj se pausa y no se
bloquea la cuenta. ADR 0007 §6: el cliente puede disputar dentro de un plazo (por ejemplo, 2 horas desde el aviso); una
disputa sobre un recargo sirve de apelación y puede acordarse reasignar el pedido con un cobro reducido; una apelación
resuelta cuenta para la reincidencia.

**Qué hay que especificar antes de programar:** estados de una disputa (abierta, resuelta a favor, en contra); quién la
resuelve; si el plazo corre desde el aviso (depende de F2); qué significa «cobro reducido» y cuánto; cómo se reasigna el
pedido (depende de F6); y si una disputa abierta también pausa la reincidencia.

**Diseño provisional.** Tabla `surcharge_disputes`; `BalanceOverdue` ignora las obligaciones con disputa abierta;
endpoints del cliente (abrir) y del operador (resolver, con motivo auditado). Es la función más grande: conviene
dividirla en al menos tres PRs.

---

## Cómo seguir este documento

- Se implementa en el orden de la tabla. F1 a F3 pueden empezar ya; F4 y F5 esperan una confirmación corta; F6 a F8
  esperan decisiones de producto.
- Al terminar cada función se actualiza su estado aquí, la sección 7 del ADR 0006 y `odd/tasks/penalty-engine.md`.
