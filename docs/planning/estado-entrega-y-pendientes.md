# Estado de la entrega por rango y pendientes del proyecto

Fecha de verificación: 2026-11-08. Rama de trabajo: `fix/integration-test-database` (cambios sin commit).

## Actualización de implementación · 2026-11-13

- La base local `last_mile_test` se prepara con `make test-db` o al ejecutar `make test-integration`. La suite de zonas usa esquemas aislados y se agregó a CI.
- Un reintento del mismo resultado y motivo reutiliza la primera evidencia de la parada. El recargo y la promoción de la cola se procesan al cerrar el pedido.
- Se registra una entrega fuera de horario con motivo. La ventana usa `OPERATING_TIMEZONE` y el radio sigue siendo obligatorio para `DELIVERED`.
- El repartidor puede mover una parada activa al comienzo con motivo auditado. Se bloquea el cambio si vuelve inalcanzable una ventana que la secuencia vigente todavía puede cumplir. La estimación incluye cinco minutos de servicio por parada.
- `WRONG_ADDRESS` requiere confirmación o dispensa de un operador, incluso si el GPS está dentro del radio. El panel está en Excepciones. El operador puede corregir un pedido aún sin asignar desde Pedidos, sin cargo. Para una parada cerrada por dirección incorrecta puede reprogramar el pedido antes de resolver el recargo. Se congela la tarifa fija de viaje configurada por ADMIN y se suma al mismo recargo al confirmarlo. Si se dispensa, no se cobra.
- La prueba HTTP contra `last_mile_test` confirmó el login de cliente y repartidor, posición en el seguimiento, paradas anteriores, radio, reintento y justificación fuera de horario. OSRM 5.27.1 con un mapa sintético devolvió geometría y ETA, y el orden persistido cambió las paradas anteriores del cliente. Sigue pendiente el recorrido con el mapa completo de Ecuador y un dispositivo. El informe del 8 de noviembre se conserva debajo como registro histórico.
- Pasaron las suites unitarias de Go y las de integración de Auth, Orders, Drivers y Zones en `last_mile_test`. Pasaron `buf lint`, `buf breaking`, 35 pruebas web de Playwright con Gateway simulado y 335 pruebas de la app móvil.

## 1. Incidente de datos: pedidos borrados en la base de desarrollo

- Ejecutar `make test-integration` contra la base de desarrollo `last_mile` ejecutó `TRUNCATE ... orders, customers`, que es borrado físico. Resultado: `orders.orders` quedó en 0 filas.
- La tabla `orders` no tenía borrado lógico. Desde esta fecha existe la columna `deleted_at` (migración `000025_orders_soft_delete`).
- Respaldo tomado antes de cualquier otro cambio, fuera del repositorio, en `C:\Users\erick\Backups\pikka-20261108-123159`:
  - `last_mile.dump`: `pg_dump -Fc` de la base de desarrollo (67 KB).
  - `postgres-data-volume.tar.gz`: copia física del volumen `last-mile-logistics_postgres-data` (18 MB), tomada con el contenedor detenido y reiniciado después.
  - `SHA256SUMS.txt`: sumas de verificación de ambos archivos.
- Pendiente: decidir si se intenta recuperar filas desde la copia física del volumen (análisis forense, sin garantía).

## 2. Borrado lógico de pedidos

- Migración `000025_orders_soft_delete` (up y down): agrega `orders.deleted_at TIMESTAMPTZ NULL` e índice parcial `WHERE deleted_at IS NULL`.
- Lecturas que excluyen pedidos borrados (`deleted_at IS NULL`):
  - `order_repository.go`: `ListByCustomer`, `GetForCustomer`, `ListAll`, `ListExceptions`, y los `SELECT ... FOR UPDATE` de cancelación, revisión y avance de parada.
  - `order_route_assignment.go`: lectura de pedidos pendientes y bloqueo para asignar ruta.
  - `order_route_stops.go`: paradas de la ruta, posición del pedido y pedidos anteriores.
  - `order_zone_queue.go`: conteos de cupo y cola de zona.
- Lecturas que NO se modificaron, a propósito: el `JOIN` de recargos (`surcharge_repository.go`), la búsqueda del teléfono de contacto para reincidencia y la consulta de cliente de reasignación. Son registros financieros o de auditoría; un pedido borrado debe seguir siendo visible ahí.
- Pruebas: `TestPedidoBorradoLogicamenteNoSeLee` (RED confirmado antes de implementar; ahora en verde). Verifica que un pedido borrado no se lee por cliente ni en el listado, y que la fila sigue existiendo.
- Tests de limpieza: los tres `DELETE FROM orders` pasaron a `UPDATE orders SET deleted_at = now()`.
- Hecho: el `TRUNCATE` de `setup` en `catalog_repository_integration_test.go` se eliminó. Cada prueba de order-service corre en su propio esquema (`t_<uuid>`) dentro de la base de pruebas: no hay estado compartido ni borrado. La prueba de migraciones (`TestOrderMigrations`) también corre en un esquema propio.
- Para aislar las pruebas hizo falta: ruta única por prueba (`testRoute` se genera en `setup`), referencias de cobro únicas (`uniqueRef`), y conteos limitados a los datos de cada prueba, porque el esquema trae el catálogo semilla de las migraciones 000009 y 000010.
- Permiso concedido solo en la base de pruebas: `GRANT CREATE ON DATABASE last_mile_test TO order_service_user`. No se concedió en `last_mile`.
- Resultado: `go test -tags integration ./...` de order-service en verde contra `last_mile_test`.

## 3. Pruebas de integración sin tocar la base de desarrollo

- Base desechable `last_mile_test` en el mismo contenedor PostgreSQL, con postgis, esquemas por servicio y permisos equivalentes al script de inicialización.
- Guard en auth-service, order-service (dos paquetes) y driver-service (dos paquetes): un `TestMain` rechaza cualquier DSN cuyo nombre de base sea `last_mile` antes de migrar o truncar.
- `make test-integration` usa `TEST_DB ?= last_mile_test`.
- Resultados contra `last_mile_test`:
  - auth-service: todos los paquetes en verde.
  - order-service: todos los paquetes en verde, incluida la nueva prueba de borrado lógico.
  - driver-service: todos los paquetes en verde.
- Las bases `auth_it`, `it_auth`, `it_orders` e `it_drivers` ya existían en el contenedor. Sin revisar. Pendiente: confirmar si el equipo las usa.

## 4. Entrega dentro y fuera del rango (verificado en vivo)

Entorno: `order-service`, `driver-service` y `api-gateway` compilados y ejecutados localmente, apuntando a `last_mile_test` (nunca a `last_mile`). Datos sembrados solo en la base de pruebas: un repartidor `ON_ROUTE` en `route-live-1`, con pedidos `IN_TRANSIT` en Quito y ventana de todo el día, para aislar la prueba del radio. Token de prueba HS256 firmado con el `JWT_SECRET` local.

| Caso | Resultado | Verificación |
|---|---|---|
| DELIVERED a ~8 m del pedido | HTTP 200, pedido `DELIVERED` | `within_radius = t`, distancia 7.9 m |
| DELIVERED a ~60 m | HTTP 422 `FAILED_PRECONDITION` | Rechazado antes de registrar |
| DELIVERED a ~500 m | HTTP 422 `FAILED_PRECONDITION` | Rechazado antes de registrar |
| NOT_DELIVERED a ~2 km (motivo `CLIENT_ABSENT`) | HTTP 200 | Registrado con `within_radius = f`, distancia 2048.2 m |
| NOT_DELIVERED en una parada ya registrada | HTTP 409 | Duplicado: `deliveries` admite un resultado por parada |

Hallazgos:
- El radio de 50 m (`DRIVER_ARRIVAL_RADIUS_METERS`) funciona de punta a punta solo para `DELIVERED`. Un `NOT_DELIVERED` siempre se registra, como indica D1.
- Un `DELIVERED` rechazado por radio no cambia el estado del pedido.
- El 409 de la tercera fila no es del radio: es la regla de un resultado por parada.
- El cambio de estado se verificó en la primera entrega dentro del rango: el pedido pasó a `DELIVERED` sin intervención. Después, el arnés de prueba reinició el pedido a `IN_TRANSIT` para repetir los casos negativos; ese reinicio fue manual y no es comportamiento de la aplicación. El pedido de la primera parada quedó en `IN_TRANSIT` solo por ese reinicio.
- La ventana horaria no se probó fuera de rango (todas las pruebas usan un día completo). Pendiente.
- El mensaje de error del móvil para `FAILED_PRECONDITION` sigue sin texto; el repartidor ve "Ocurrió un error inesperado". Pendiente.

## 5. Pendientes del feature customer-driver-map (según `odd/tasks/customer-driver-map.md`)

- T5: verificación con el stack levantado. Parcial: el flujo de resultado sí se probó en vivo (sección 4); faltan `buf lint`, `buf breaking` y la verificación del mapa del cliente.
- P2: prueba en vivo de OSRM (`make osrm-up`), con geometría y orden de paradas reales.
- P3: reordenar paradas. Ver sección 6.
- P5: UI del repartidor para reordenar. Ver sección 6.
- P6: dirección incorrecta en zona aceptada: confirmación del operador con el cliente.
- P7: estado `PENDING_VERIFICATION` para recargos por `WRONG_ADDRESS`.

Otros documentos con tareas abiertas: `driver-location-tracking` (T9), `driver-optimal-route` (T7), `driver-route-read` (T5), `order-route-assignment` (T4) y `routing-plan-generation` (T5).

## 6. Ordenamiento de entregas: factores a considerar para la implementación

El sistema calcula el orden de las paradas por prioridad y distancia (OSRM). Cuando el repartidor reordena, o cuando una parada se retrasa, el reordenamiento debe considerar:

1. **Ventana horaria de cada pedido** (`window_start`, `window_end`): una parada que se pasa de la ventana pierde valor, y el orden debe priorizar las ventanas que vencen primero.
2. **Riesgo de retraso**: estimación de llegada (ETA) por parada frente al fin de su ventana. Una parada con ETA fuera de ventana debe marcarse antes de que ocurra, no después.
3. **Tiempo de servicio por parada**: el tiempo en el sitio cambia el ETA de las siguientes paradas; no basta con la distancia.
4. **Costo de reordenar**: cada cambio debe recalcular OSRM y debe ser visible para el cliente cuyo `stops_before` cambie (decisión D2).
5. **Pedidos que terminan sin entrega**: un `NOT_DELIVERED` libera la ruta, pero el cliente puede necesitar reasignación. El orden restante debe recalcularse al cerrar la parada, no al final de la ruta.
6. **Reasignación antes del despacho** (D3, D5): si el cliente cambia de dirección o de fecha antes de que el pedido se asigne, la parada sale de la ruta sin cargo; si ya va en camino, aplica el cargo de viaje.
7. **Prioridad del negocio**: pedidos marcados como urgentes o de último momento no deben quedar al final por la optimización de distancia.
8. **Decisión del repartidor frente al sistema**: el repartidor puede corregir el orden, pero el sistema debe registrar el cambio y el motivo para auditoría, sin dejar que una corrección manual rompa una ventana ya comprometida.

Pendiente de decisión del usuario: si la optimización debe tratar la ventana como restricción dura (la parada no se puede saltar) o como penalización (la parada se puede retrasar con costo).

## 7. Qué NO está completo

- No se afirma que el proyecto esté 100% completo. Hay pendientes en los documentos listados en la sección 5.
- No se ejecutaron las suites de Playwright E2E.
- `golangci-lint` (config del repo) en auth-service, driver-service y order-service: 0 hallazgos.
- SonarCloud (proyecto `ErickGuerron_Backend-Pikka`, lectura pública del último análisis de `develop`): quality gate OK; bugs 0; vulnerabilidades 0; code smells 0; hotspots 0; cobertura 80.9 %; duplicación 4.1 %. Este estado NO incluye los cambios de esta rama. Para analizarlos hace falta `SONAR_TOKEN` y `SONAR_HOST_URL`, que no están en este entorno; el job del CI solo corre con `vars.SONAR_ENABLED=true`.
- No se hizo commit. Los cambios están en la rama `fix/integration-test-database`.

## 8. Borrados físicos que quedan (decisión pendiente)

- Resuelto: `auth-service` y `driver-service` ya no hacen `TRUNCATE`. Cada prueba usa su esquema propio (`integration_schema_test.go`). Las pruebas que contaban el total de filas ahora miden contra la semilla de la migración 000003 o filtran por sus propios ids (`onlyIDs`, en `driver_seed_test.go`). En auth, la prueba de ADMIN único desactiva la semilla solo dentro de su esquema. Permiso `CREATE` concedido solo en `last_mile_test`.
- `order-service` (pruebas): `DELETE FROM customers` y `DELETE FROM customer_notifications` en limpieza. No son pedidos; no se tocaron.
- `zone-service` (CÓDIGO DE PRODUCCIÓN): `zone_repository.go:187` ejecuta `DELETE FROM zones`. Es un borrado físico de la aplicación, no de pruebas. Requiere decisión: ¿zonas con borrado lógico igual que pedidos?
- Pruebas de zone-service también hacen `DELETE FROM zones` en limpieza.
