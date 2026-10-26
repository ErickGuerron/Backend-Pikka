# Cola de espera por cupo de zona (zone-capacity-queue)
Objetivo: cuando la zona de un pedido no tiene cupo, el pedido queda en cola FIFO y se asigna cuando hay capacidad; el cliente ve su lugar en la cola y el repartidor ve el cupo de su zona.
Ramas: backend `feat/zone-capacity-queue` (este documento, PR abierta hacia `develop`); frontend `feat/customer-queue-message` (PR abierta hacia `develop`, su parte esta en `odd/tasks/zone-capacity-queue.md` de Frontend-Pikka).
Entrega: PRs abiertas como respaldo para revision del equipo. Merge y delivery siguen la politica normal del repositorio; no se hace merge sin decision explicita.
Test runner: `go test ./...` (unitarias, dentro de `services/order-service`), `go test -tags integration -p 1 ./...` (integracion, PostgreSQL/PostGIS), `golangci-lint`, `gofmt`, `buf lint` si cambia proto. Frontend: `npx tsc -b --noEmit`, `npx vitest run` (web); `npx tsc --noEmit`, `npx jest -w 1` (mobile); `npx playwright test -c playwright.mobile.config.ts` (e2e movil, no ejecutado en esta sesion).

## Contexto para el equipo y para las IAs
- Que es la cola: un pedido nuevo en una zona sin cupo para su fecha no recibe ruta. Queda en estado PENDING con una posicion FIFO. Cuando un pedido activo libera cupo (entrega, cancelacion o cierre de parada) o cuando el barrido corre, se le asigna una ruta.
- Estados que cuentan para el cupo: ROUTE_ASSIGNED, PICKED_UP, IN_TRANSIT. Los PENDING no ocupan cupo; esperan.
- Donde vive la logica: `services/order-service/internal/application/order/zone_queue.go` (caso de uso `ZoneQueue`, con `Fill`), su puerto en `application/ports/zone_queue.go`, la implementacion SQL en `adapters/persistence/order_zone_queue.go` (`FillZoneQueue`), y el dominio en `domain/zone_queue.go`.
- Donde se dispara: hooks `WithZoneQueue` en `create_order.go`, `route_assignment.go`, `cancel_order.go` y `record_stop_outcome.go`; el cableado esta en `cmd/api/main.go`.
- Contratos: `contracts/proto/orders/v1/orders.proto` y `contracts/openapi/openapi.yaml`, mas `contracts/gen/go/orders/v1/orders.pb.go` regenerado.

## Specs
S1 (R1, confirmado). La capacidad de la zona es el total de pedidos activos (ROUTE_ASSIGNED, PICKED_UP, IN_TRANSIT) de esa zona y fecha, compartido por todas sus rutas. La ruta elegida es la de menos activos. Estado: implementado (180c00f).
S2 (R2, confirmado en #3581 con la regla descrita). El cliente ve al repartidor (posicion, ruta por calles, ETA) solo desde PICKED_UP hasta la entrega. Antes de PICKED_UP el mensaje es: "Tu pedido esta asignado a un repartidor; aun no sale del local", sin ruta, ETA ni numero de paradas previas. Se aplica en el gateway (`tracking_snapshot.go`), no solo en la app. Estado: pendiente.
S3 (R3, confirmado). Pedido programado, en web y movil: "Pedido programado para entregarse el <fecha> entre <ventana>. Lo asignaremos a un repartidor de tu zona ese dia." Estado: pendiente (frontend).
S4 (R4, confirmado). Edicion en espera:
  - PENDING: direccion y coordenadas editables; revalidar cobertura. Si cambia de zona sale de su cola y entra al final de la nueva. Si queda fuera de cobertura: mantener la original o cancelar.
  - PENDING y ROUTE_ASSIGNED: referencia, telefono y codigo postal editables sin efecto en la cola.
  - PENDING: fecha y ventana editables; el pedido pasa al final de la cola de la nueva fecha.
  - Productos quedan fuera de esta fase. Despues de PICKED_UP no se permite ningun cambio. Cancelar funciona como hoy.
  - Campo `queued_at` aparte si se reencola; FIFO por `created_at` original si no cambia zona ni fecha.
  Estado: pendiente.
S5 (R5, confirmado). Barrido cada 5 minutos por zona y fecha con pedidos PENDING sin ruta; llama a `ZoneQueue.Fill` (idempotente, con lock). Es red de seguridad. Estado: pendiente. `Fill` hoy se invoca desde los hooks, no desde un ticker cableado en `cmd/api/main.go`.
S6. Pantalla del repartidor "Cupo X de Y" con la capacidad total de la zona. Estado: parcial. La app movil ya muestra "Cupo: N de M pedidos" en la ruta de hoy (`apps/mobile/src/app/(app)/ruta/index.tsx`, desde b03ac5d, PR #43), pero N es `route.stops.length` y M es `route.maxOrders`: cuenta por ruta, no por zona. Eso contradice S1 (la capacidad es de la zona y es compartida). Hay que alinear el numero con el total de activos de la zona y fecha. Origen del dato de M por confirmar (Zone Service `max_orders_per_route`).
S7 (decision de producto acordada, no es R-item). El sistema recomienda el orden de paradas; el repartidor decide el orden final. El backend guarda el orden confirmado por el repartidor (no solo la sugerencia), valida restricciones de capacidad y estado en el servidor, y registra si el orden fue sugerido o modificado. Estado: sin especificar; definir antes de tocar routing.
S8. `POST /routes/plan` de routing-service debe respetar la cola: planificar solo pedidos activos o la capacidad de zona. Estado: pendiente.

## Decisiones y su justificacion
- **Capacidad por zona y fecha, no por ruta (S1).** Si el cupo fuera por ruta, dos rutas de la misma zona podrian sobrevender o dejar cupo ocioso sin que el sistema lo vea. Contar el total de activos de la zona da una sola fuente de verdad. La ruta elegida es la de menos activos para repartir la carga.
- **FIFO por `created_at` (S4).** Quien pidio primero sale primero de la cola. Reencolar cambia la posicion solo cuando el pedido cambia de zona o fecha; por eso existe `queued_at` separado: `created_at` conserva la antiguedad real del pedido y `queued_at` marca cuando entro a la cola nueva.
- **Barrido ademas de hooks (S5).** Los hooks reaccionan a eventos conocidos. El barrido cubre casos que ningun evento dispara (por ejemplo, un cupo que se libera por un cambio externo, o un fallo transitorio en un hook). Es idempotente: ejecutarlo de mas no crea rutas de mas.
- **Lock en `Fill` (S5).** Hooks y barrido pueden correr a la vez sobre la misma zona y fecha. El lock evita doble asignacion.
- **Ocultar ubicacion en el gateway, no en la app (S2).** Si la app ocultara los datos, un cliente con una peticion directa o una app vieja veria posicion antes de tiempo. La regla de privacidad vive donde se construye la respuesta.
- **R3 solo en frontend (S3).** Es texto de presentacion. No cambia contrato ni datos; por eso es la ultima pieza de frontend y no requiere backend nuevo.
- **Orden de paradas decidido por el repartidor (S7).** Es una decision de producto, no tecnica: el sistema sugiere, el repartidor manda. Por eso el backend guarda el orden confirmado y no solo la sugerencia. Se define antes de tocar routing para no reabrir el contrato dos veces.
- **Routing respeta la cola (S8) al final.** Si routing planificara pedidos que siguen en cola, duplicaria la asignacion que la cola hace. Va despues de cerrar la semantica de cupo.

## Plan y justificacion del flujo
- Clasificacion: tarea grande (varias sesiones, varios requisitos, backend y frontend). Por eso lleva este documento como tracking y espejo en Engram.
- Orden: R5 -> R4 -> R2 -> R3 -> pantalla "Cupo X de Y" -> routing (S8).
  - R5 va primero porque `Fill` ya existe y es la red de seguridad: sin barrido, un pedido PENDING puede quedar sin ruta si ningun evento dispara la asignacion.
  - R4 depende de la cola y de `queued_at`, asi que va despues de cerrar el barrido.
  - R2 es independiente del resto, en el gateway.
  - R3 es solo frontend y no bloquea backend; se entrega con la pantalla de cupo porque ambas leen el estado de cola.
  - Routing (S8) va al final porque necesita la semantica de cola ya cerrada.
- Test-first: RED observado por requisito, luego GREEN, luego refactor. R4 empieza por el contrato OpenAPI y proto antes del codigo. Cada regla de R4 (campo y estado) tiene su prueba.
- Unidades de trabajo: commits de unos 400 lineas, cada uno con codigo, pruebas y docs. Un commit por requisito pendiente.
- Riesgo esperado por unidad (Risk line): R4 (`queued_at` y reencolado tocan datos persistidos) = item 1; R4 y S8 cambian contratos OpenAPI/proto = item 3; R2 cambia la respuesta del gateway = item 3. Los demas se revisan al implementarse.
- Delivery: PRs abiertas hacia `develop`. La base previa (PR #76 y PR #43) ya esta mergeada, asi que no hay cadena de PRs apiladas.

## Tasks
| ID | Specs | Ruta | Estado | Commit / evidencia |
|---|---|---|---|---|
| T1 | S1 | inline | hecho | 180c00f: capacidad total de zona; RED observado antes del cambio (prueba con dos rutas llenas) |
| T2 | S1, S5 (parte de `Fill`) | inline | hecho parcial | 1109dab: `FillZoneQueue`, `ZoneQueue`, hooks, cableado en main; sin ticker |
| T3 | S5 | inline | pendiente | ticker de 5 min en `cmd/api/main.go` que llame `ZoneQueue.Fill` por zona y fecha con PENDING |
| T4 | S4 | inline | pendiente | contrato OpenAPI/proto primero; endpoint de edicion; revalidacion de cobertura; `queued_at` |
| T5 | S2 | inline | pendiente | `tracking_snapshot.go` oculta posicion, ruta, ETA y `stops_before` si estado < PICKED_UP |
| T6 | S3 | inline (frontend) | pendiente | texto de programados en web y movil |
| T7 | S6 | inline (movil + backend) | parcial | movil ya muestra "Cupo: N de M" por ruta (b03ac5d); falta total por zona y origen de M (Zone Service `max_orders_per_route`) |
| T8 | S8 | inline | pendiente | `POST /routes/plan` respeta la cola |
| T9 | S7 | inline | pendiente | requiere definicion de producto antes de implementar |

## Implementado parcialmente y lo que falta
Implementado y verificado:
- 1109dab: contratos proto y OpenAPI, `FillZoneQueue` en persistencia, aplicacion `ZoneQueue` con hooks en creacion, asignacion de ruta, cancelacion y cierre de parada; cableado de `ZoneQueue` en `cmd/api/main.go`; pruebas de caso de uso, de dominio y de handler gRPC.
- 180c00f: capacidad total de zona compartida por rutas (R1), con prueba de integracion `TestZoneQueueCapacityIsZoneTotalAcrossRoutes`.
- Verificado al cerrar esta revision: `go test -count=1 ./...` dentro de `services/order-service` en verde (todos los paquetes con pruebas: adapters/geocoding, grpc, zones; application/catalog, customer, order; domain).
- Pendiente de ejecutar en esta revision: la suite de integracion (`-tags integration -p 1`). `TestOrderMigrations` falla por entorno (usuario superusuario, no aislado); es esperado segun #3581.

Falta respecto a implementaciones anteriores:
- R5 (barrido) no esta cableado en `main.go`. Hoy la cola depende solo de hooks.
- Routing `POST /routes/plan` no respeta la cola (S8). Riesgo: planificar sobre pedidos que deberian esperar.
- Pantalla "Cupo X de Y" (S6): existe una version por ruta en movil que cuenta paradas de la ruta, no activos de la zona. Falta alinearla con R1 y definir el origen del maximo.
- R2 (gateway) y R4 (edicion) no tienen codigo.
- Frontend: 33ea1ee muestra el lugar en la cola (web y movil). R3 no esta.
- `Plan-de-trabajo-Pikka.md` esta desactualizado respecto a develop (memoria #3579).
- Worktrees temporales en `AppData\Local\Temp\opencode` y `pikka-*` pendientes de poda.

## Mapa de archivos (rama feat/zone-capacity-queue)
- `services/order-service/internal/application/order/zone_queue.go`: caso de uso `ZoneQueue`, `Fill` y su pruebas en `zone_queue_test.go` y `zone_queue_hooks_test.go`.
- `services/order-service/internal/adapters/persistence/order_zone_queue.go`: `FillZoneQueue` y `routeWithRoom` (elige la ruta de menos activos). Prueba: `order_zone_queue_integration_test.go`.
- `services/order-service/internal/domain/zone_queue.go`: reglas de dominio de la cola y su prueba.
- `services/order-service/internal/adapters/grpc/orders_server.go` y `adapters/http/orders_handlers.go`: exposicion de la cola en la API.
- `services/order-service/cmd/api/main.go`: cableado de `ZoneQueue` (no hay ticker todavia).
- `contracts/proto/orders/v1/orders.proto`, `contracts/openapi/openapi.yaml`, `contracts/gen/go/orders/v1/orders.pb.go`.

## Como verificar y retomar (personas e IAs)
1. Leer este documento completo antes de tocar codigo; verificar que la rama esta en el commit esperado (`git log --oneline -3`).
2. Unitarias: `cd services/order-service; go test -count=1 ./...`.
3. Integracion: levantar PostgreSQL/PostGIS (contenedor `pikka-test-pg`, puerto 55433, imagen `postgis/postgis:16-3.4`), crear los esquemas `orders, auth, zones, routing, drivers, tracking` y usar el DSN de prueba configurado en el entorno local; luego `go test -tags integration -p 1 ./...`. El fallo de `TestOrderMigrations` por superusuario es conocido.
4. Para cada requisito pendiente: escribir primero la prueba (RED), observar que falla por la razon esperada, implementar (GREEN), luego refactor.
5. Antes de commitear, revisar `Risk:` de la unidad (ver arriba) y el diff contra la base.
6. No hacer push, PR ni merge sin permiso explicito del usuario.

## Preguntas abiertas
- Origen del dato "Cupo X de Y": confirmar si viene de Zone Service (`max_orders_per_route`) o de otra fuente.
- S7: definir como se guarda el orden confirmado (campo, historial, validacion de capacidad) antes de tocar routing.
- `queued_at`: confirmar migracion para pedidos existentes (backfill desde `created_at`).

## Log
L1 (origen, reconstruido desde Engram #3579, #3580, #3581; no es texto literal del usuario, que no quedo guardado): el usuario pidio cola de espera por cupo de zona en Pikka, confirmo R1, R3, R4 y R5, y confirmo R2 con la regla descrita. Pidio continuar la implementacion, documentar lo faltante, el plan, la justificacion del flujo y las implementaciones parciales, y subir la rama.
L2: se documenta el estado real verificado en git y se publica la rama como respaldo. Sin merge.
L3: se abren PRs hacia `develop` para revision del equipo. Se amplia la documentacion con decisiones, mapa de archivos, guia para retomar y preguntas abiertas. Verificacion: unitarias de order-service en verde; integracion pendiente.
