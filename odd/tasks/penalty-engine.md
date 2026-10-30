# Motor de penalizaciones y cobro simulado (penalty-engine)
Objetivo: implementar lo que el ADR 0006 decide y hoy no existe en codigo: recargo del 15 %, cobro simulado (`PaymentGateway`, `payment_attempts`), verificacion por `WRONG_ADDRESS`, cobro por reasignacion, reincidencia, bloqueo por saldo y pago del saldo.
Rama: `feat/penalty-engine` (backend). Frontend en una rama aparte al llegar a T10.
Entrega: commits por unidad de trabajo; PR hacia `develop` solo con decision explicita. Estrategia: `ask-on-risk`.
Test runner: `go test ./...` y `go test -tags integration -p 1 ./...` dentro de `services/order-service`; `golangci-lint`, `gofmt`, `buf lint` si cambia proto.

## Contexto para el equipo y para las IAs
- Fuente normativa: `docs/adr/0006-recargo-por-rechazo-con-cobro-simulado.md` (y ADR 0005, 0007 en lo que cita).
- Ya existe: `customers.pending_surcharge NUMERIC(12,2)` (migracion 000001), estado `BLOQUEADO_POR_SALDO` (`domain/customer_status.go`, `update_status.go`). Nada escribe el saldo ni activa el bloqueo.
- No existe: `PaymentGateway`, `payment_attempts`, recargo, `PENDING_VERIFICATION`, reincidencia, reloj de 2 semanas, pago del saldo, nada en frontend.
- Convenciones: dinero siempre `decimal.Decimal` (nunca float); migraciones numeradas con `.up.sql`/`.down.sql`; puertos en `internal/application/ports`; SQL en `internal/adapters/persistence`.

## Specs
Cada spec cita el ADR 0006; el texto del ADR manda.
S1. "Order Service define una interfaz `PaymentGateway` con operaciones de cobro y reembolso." Adaptador de simulacion "sin llamadas a red ni a un proveedor externo".
S2. "Cada intento de cobro se registra en la tabla `payment_attempts` con monto, estado (`SIMULATED_APPROVED` o `SIMULATED_DECLINED`), metodo (`CASH` o `CARD`), referencia interna y fecha."
S3. "El sistema **no pide, no almacena y no registra** numero de tarjeta, vencimiento ni CVV."
S4. "Monto: 15 % del precio base del siguiente pedido del cliente." "Una sola vez por incidencia."
S5. WRONG_ADDRESS: el recargo pasa a `PENDING_VERIFICATION`; "Ningun cobro simulado se registra en este estado." El operador valida.
S6. Cobro adicional por reasignacion: "calculado por distancia con el precio de la gasolina como referencia y ajustable por un administrador".
S7. Reincidencia: "Una tercera dentro de ese plazo marca al cliente como reincidente y el operador puede aplicar el cobro con justificacion, sujeto a auditoria."
S8. Saldo: `customers.pending_surcharge` acumula; "aparece como linea separada en el siguiente pedido ... antes de que el pedido se confirme"; marcado como simulado.
S9. Bloqueo: pasadas 2 semanas sin pagar, `BLOQUEADO_POR_SALDO`; no puede crear ni cancelar pedidos; "Disputa abierta: ... el reloj se pausa".
S10. Pago del saldo desde la app con cobro simulado; "al pagar el saldo completo, la cuenta vuelve a `ACTIVO` en ese mismo momento."
S11. Pruebas obligatorias: "el recargo es exactamente 15 % del precio base y se aplica una sola vez por incidencia; el cobro simulado nunca se presenta como cargo real; ningun log, respuesta ni tabla contiene datos de tarjeta."

## Decisiones abiertas (bloquean tareas concretas, ver Tasks)
- D1. Lista cerrada de justificaciones validas del rechazo (T3).
- D2. Pago "sin cuenta" que cita el ADR 0005 y que el ADR 0006 no define (T0).
- D3. Parametros de S6: formula de distancia, precio de gasolina, donde se guarda lo que ajusta el administrador (T5).
- D4. Que pasa si el operador rechaza un `PENDING_VERIFICATION` y cuanto tiempo puede quedar pendiente (T4).
- D5. Separar en el ADR los dos "2 semanas": plazo de reincidencia y plazo de bloqueo por saldo (T0, T6, T7).

## Tasks
| ID | Specs | Ruta | Estado | Depende de | Riesgo |
|---|---|---|---|---|---|
| T0 | D2, D5 + estado del ADR | inline | pendiente | decision de producto | ninguno (docs) |
| T1 | S1, S2, S3 | inline | hecho (integracion sin ejecutar: Docker apagado) | - | item 1 (migracion nueva, tabla nueva; no toca datos existentes) |
| T2 | S4 | inline | hecho (integracion sin ejecutar) | T1 | item 1 (tabla `surcharges`) |
| T3 | S4 | inline | hecho (integracion sin ejecutar) | T2 | item 3 (campo opcional `reason` en proto; aditivo) |
| T4 | S5 | inline | hecho (integracion sin ejecutar) | T2, D4 | item 3 (RPC y rutas nuevas de operador; aditivo) |
| T5 | S6 | inline | pendiente | T2, D3 | item 1 si guarda parametros |
| T6 | S7 | inline | pendiente | T2, D5 | ninguno |
| T7 | S9 | inline | pendiente | T2, D5 | item 4 (reloj y concurrencia con disputas) |
| T8 | S10 | inline | pendiente | T1, T7 | item 3 (endpoint nuevo) |
| T9 | S8 | inline | pendiente | T2 | item 3 (campo nuevo en pedido) |
| T10 | S8, S9, S10 | frontend | pendiente | T8, T9 | ninguno |
| T11 | S11 + docs (ADR, `orders-v1.md`, README, OpenAPI) | inline | pendiente | todas | ninguno |

Orden: T1 -> T2 -> (T3, T4) -> T6 -> T5 -> T7 -> T8 -> T9 -> T10 -> T11. T0 se resuelve cuando el usuario decida D2/D5; no bloquea T1 ni T2.

## Log
L1: "hagamos un plan en pasos para implementar cada una de estas cosas que faltan y de alli empecemos" (tras revisar el ADR 0006 contra el codigo).
L2: se crea la rama `feat/penalty-engine` y este documento. Se empieza por T1 porque no depende de ninguna decision abierta.
L3: T1 implementada. RED observado (indefinidos en domain y payments), GREEN con `go test ./...` en verde. `go vet -tags integration` limpio. Las pruebas de integracion de `payment_attempts` estan escritas pero NO se ejecutaron: el daemon de Docker no estaba activo y no hay `ORDER_TEST_DATABASE_URL`. Pendiente correrlas (postgis/postgis:16-3.4) antes del PR.
L4: T2 implementada. Decision D6: el ADR dice "15 % del precio base del siguiente pedido", que no existe cuando ocurre la incidencia; por eso `surcharges` guarda la incidencia PENDING sin monto (`order_id` UNICO = una sola vez por incidencia) y `SurchargeFor` calcula el 15 % cuando haya siguiente pedido (T9). Hallazgo: `Customer.AddSurcharge/PaySurcharge/BlockBySurcharge/Unblock` ya existen en `domain/customer.go`; T7 y T8 deben reutilizarlos. Integracion de `surcharges` escrita y NO ejecutada (Docker apagado).
L5: T3 implementada. D1 queda resuelta por ADR 0007 seccion 4-5: recargo automatico solo para CLIENT_REFUSED, CLIENT_ABSENT y OUT_OF_WINDOW (`NotDeliveredReason.ChargesSurcharge`); motivos del negocio no cobran; WRONG_ADDRESS va a verificacion (T4). Se agrega `reason = 4` a `RecordRouteStopOutcomeRequest` (el gateway ya lo recibia y solo lo mandaba a driver-service). La incidencia se registra DESPUES de cerrar la parada y su fallo se loguea sin deshacer el cierre (el repartidor no puede reintentar una parada cerrada; `order_id` unico hace seguro reintentar a mano). Riesgo conocido: si el registro falla, la penalidad se pierde hasta que alguien la reintente; mitigar con barrido de reconciliacion (anotar para T7). Hallazgo: driver-service clasifica WRONG_ADDRESS como BUSINESS, pero la enmienda del ADR 0006 pide verificacion; T4 debe resolverlo. No se valida aqui la llegada dentro de ventana/GPS: la valida driver-service antes. Cambio compatible: sin `reason` no hay incidencia.
L6: T4 implementada en 3 commits. (1) `WRONG_ADDRESS` registra la incidencia en `PENDING_VERIFICATION` y sin cobro. (2) `Surcharge.Resolve` + `ResolveSurchargeVerification`: el operador decide CONFIRM (vuelve a PENDING, sigue el flujo normal) o WAIVE (WAIVED), con motivo de 5 a 300 caracteres, operador y hora; fila bloqueada con `FOR UPDATE` para que dos operadores no resuelvan la misma; migracion 000016. (3) Transporte: RPC `ListSurchargesPendingVerification` y `ResolveSurchargeVerification`; rutas `GET /api/v1/surcharges/pending-verification` y `POST /api/v1/surcharges/{id}/verification` (solo ADMIN/OPERATOR, `StaffOnly`); OpenAPI y tag `surcharges`. El operador auditado sale del JWT, no del cuerpo. Decision D4 (asumida, a confirmar con el equipo): rechazar la verificacion = WAIVE; no hay plazo maximo en verificacion porque el recargo aun no tiene monto ni reloj. Documentada en ADR 0006 seccion 1. Se usa una interfaz `SurchargeClient` separada de `OrderClient` para no tocar los fakes existentes. Pendiente: driver-service sigue clasificando WRONG_ADDRESS como BUSINESS (solo etiqueta; order-service decide por su propia lista). Integracion (resolver, concurrencia, cola) escrita y NO ejecutada: Docker apagado.
