# Cola de espera por cupo de zona (zone-capacity-queue)
Objetivo: cuando la zona de un pedido no tiene cupo, el pedido queda en cola FIFO y se asigna cuando hay capacidad; el cliente ve su lugar y el repartidor ve el cupo de su zona.
Ramas: backend `feat/zone-capacity-queue` (este documento); frontend `feat/customer-queue-message` (ver `odd/tasks/zone-capacity-queue.md` en Frontend-Pikka).
Entrega: pendiente de decision del usuario. La base previa (PR #76) ya esta mergeada en GitHub; la PR debe apuntar a `develop` tras sincronizar. Push hecho solo para respaldo; sin PR ni merge sin permiso explicito.
Test runner: `go test ./...` (unitarias), `go test -tags integration -p 1 ./...` (integracion, PostgreSQL/PostGIS), `golangci-lint`, `gofmt`, `buf lint` si cambia proto. Frontend: typecheck, jest -w 1, vitest --no-file-parallelism, playwright mobile.

## Specs
S1 (R1, confirmado). La capacidad de la zona es el total de pedidos activos (ROUTE_ASSIGNED, PICKED_UP, IN_TRANSIT) de esa zona y fecha, compartido por todas sus rutas. La ruta elegida es la de menos activos. Estado: implementado (180c00f).
S2 (R2, confirmado en #3581 con la regla descrita). El cliente ve al repartidor (posicion, ruta por calles, ETA) solo desde PICKED_UP hasta la entrega. Antes de PICKED_UP el mensaje es: "Tu pedido esta asignado a un repartidor; aun no sale del local", sin ruta, ETA ni numero de paradas previas. Se aplica en el gateway (tracking_snapshot.go), no solo en la app. Estado: pendiente.
S3 (R3, confirmado). Pedido programado, en web y movil: "Pedido programado para entregarse el <fecha> entre <ventana>. Lo asignaremos a un repartidor de tu zona ese dia." Estado: pendiente (frontend).
S4 (R4, confirmado). Edicion en espera:
  - PENDING: direccion y coordenadas editables; revalidar cobertura. Si cambia de zona sale de su cola y entra al final de la nueva. Si queda fuera de cobertura: mantener la original o cancelar.
  - PENDING y ROUTE_ASSIGNED: referencia, telefono y codigo postal editables sin efecto en la cola.
  - PENDING: fecha y ventana editables; el pedido pasa al final de la cola de la nueva fecha.
  - Productos quedan fuera de esta fase. Despues de PICKED_UP no se permite ningun cambio. Cancelar funciona como hoy.
  - Campo `queued_at` aparte si se reencola; FIFO por `created_at` original si no cambia zona ni fecha.
  Estado: pendiente.
S5 (R5, confirmado). Barrido cada 5 minutos por zona y fecha con pedidos PENDING sin ruta; llama a `ZoneQueue.Fill` (idempotente con lock). Es red de seguridad. Estado: pendiente. `Fill` hoy se invoca desde hooks de creacion y asignacion, no desde un ticker cableado en `cmd/api/main.go`.
S6. Pantalla del repartidor "Cupo X de Y" con la capacidad total de la zona. Origen del dato por confirmar (Zone Service `max_orders_per_route`). Estado: pendiente.
S7 (decision de producto acordada, no es R-item). El sistema recomienda el orden de paradas; el repartidor decide el orden final. El backend guarda el orden confirmado por el repartidor (no solo la sugerencia), valida restricciones de capacidad y estado en el servidor, y registra si el orden fue sugerido o modificado. Estado: sin especificar; definir antes de tocar routing.
S8. `POST /routes/plan` de routing-service debe respetar la cola: planificar solo pedidos activos o la capacidad de zona. Estado: pendiente.

## Plan y justificacion del flujo
- Clasificacion: tarea grande (varias sesiones, varios requisitos, backend y frontend). Por eso lleva este documento como tracking y espejo en Engram.
- Orden: R5 -> R4 -> R2 -> R3 -> pantalla "Cupo X de Y" -> routing (S8). R5 va primero porque `Fill` ya existe y sin barrido un pedido PENDING puede quedar sin ruta si nadie dispara la asignacion. R4 depende de la cola y de `queued_at`. R2 es independiente en el gateway. R3 es solo frontend y se hace al final del backend que lo respalda. Routing (S8) va al final porque necesita la semantica de cola ya cerrada.
- Test-first: RED observado por requisito, luego GREEN. R4 empieza por el contrato OpenAPI y proto antes del codigo. Cada regla de R4 (campo y estado) tiene su prueba.
- Unidades de trabajo: commits de unos 400 lineas, cada uno con su codigo, pruebas y docs. Un commit por requisito pendiente.
- Riesgo esperado por unidad (Risk line): R4 (`queued_at` y reencolado tocan datos persistidos) = item 1; R4 y S8 cambian contratos OpenAPI/proto = item 3; R2 cambia la respuesta del gateway = item 3. Los demas se revisan al implementarse.
- Delivery: decision pendiente del usuario (`ask-on-risk` por defecto). Feature en cadena: PR #76 ya mergeada, por lo que la base real es `develop`.

## Tasks
| ID | Specs | Ruta | Estado | Commit / evidencia |
|---|---|---|---|---|
| T1 | S1 | inline | hecho | 180c00f (sin push antes de este documento) |
| T2 | S1, S5 (parte de `Fill`) | inline | hecho parcial | 1109dab: `FillZoneQueue`, `ZoneQueue`, hooks en creacion/asignacion/cancelacion; sin ticker |
| T3 | S5 | inline | pendiente | - |
| T4 | S4 | inline | pendiente | - |
| T5 | S2 | inline | pendiente | - |
| T6 | S3 | inline (frontend) | pendiente | - |
| T7 | S6 | inline | pendiente | origen del dato por confirmar |
| T8 | S8 | inline | pendiente | - |
| T9 | S7 | inline | pendiente | requiere definicion antes de implementar |

## Implementado parcialmente y lo que falta
Implementado:
- 1109dab: contratos proto y OpenAPI, `FillZoneQueue` en persistencia, aplicacion `ZoneQueue` con hooks en CreateOrder, asignacion de ruta y cancelacion/cierre de parada; cableado de `ZoneQueue` en `cmd/api/main.go`.
- 180c00f: capacidad total de zona compartida por rutas (R1).
- Verificado al documentar: `go test ./internal/application/... ./internal/domain/...` en verde.
- Integracion: `TestOrderMigrations` falla por entorno (usuario superusuario, no aislado); esperado segun la nota de #3581. Las demas pruebas de cola de integracion no se re-ejecutaron en esta sesion.

Falta respecto a implementaciones anteriores:
- R5 (barrido) no esta cableado en `main.go`.
- Routing `POST /routes/plan` no respeta la cola (S8).
- Pantalla "Cupo X de Y" y origen del dato (S6).
- R2 (gateway) y R4 (edicion) no tienen codigo.
- Cola en frontend: 33ea1ee muestra el lugar en la cola (web y movil); R3 no esta.
- Local `develop` esta desactualizado frente a `origin/develop` (0a4a2d6); sincronizar antes de abrir la PR.
- `Plan-de-trabajo-Pikka.md` esta desactualizado respecto a develop (memoria #3579).
- Worktrees temporales en `AppData\Local\Temp\opencode` y `pikka-*` pendientes de poda.

## Log
L1 (origen, reconstruido desde Engram #3579, #3580, #3581; no es texto literal del usuario, que no quedo guardado): el usuario pidio cola de espera por cupo de zona en Pikka, confirmo R1, R3, R4 y R5, y confirmo R2 con la regla descrita. Pidio continuar la implementacion y, despues, documentar lo faltante, el plan, la justificacion del flujo y las implementaciones parciales, y subir la rama.
L2: se documenta el estado real verificado en git y se publica la rama como respaldo. Sin PR, sin merge.
