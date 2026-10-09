# División de tareas por sprints (2 semanas)

Equipo: **Erick** (Full stack), **Anthony** (Backend), **Xabier** (Frontend), **Mabe** (Full stack).
Base: `docs/architecture/BASE_TECNICA.md`, secciones 46 (fases) y 31–35 (pruebas, seguridad, calidad).

Las fechas de inicio se definen al arrancar el Sprint 1. Cada sprint dura 2 semanas.

Este documento cubre el backend. Las tareas de frontend (web y app móvil) están en `Frontend-Pikka/docs/planning/division-de-tareas.md`. El plan completo y consolidado para entregar en Word está en `Proyecto-Uber-Eats/Plan-de-trabajo-Pikka.md`.

## Cómo se trabaja

- **Contrato primero:** el `.proto` y el OpenAPI de un servicio se mergean antes que su implementación. `buf breaking` debe pasar.
- **Un PR = un alcance pequeño** (ver sección 44). Cada PR lo revisa otra persona del equipo.
- **Quality gate por PR:** compila, `make test`, `make lint`, `make openapi-lint`, tests de integración si toca base de datos, sin secretos.
- **Pruebas de seguridad y rendimiento** (sección "Pruebas OWASP y rendimiento") corren al cierre de cada sprint.

## Pruebas OWASP y rendimiento: contenedores

| Herramienta | Comando | Qué hace | Cuándo corre |
|---|---|---|---|
| k6 | `make perf-smoke` | Smoke de carga mínima sobre `/health` y `/ready` (`test-data/performance/smoke.js`) | Cada sprint y antes de cada entrega |
| OWASP ZAP | `make security-scan` | Escaneo activo de la API a partir de `openapi.yaml` (`zap-api-scan.py`) | Cada sprint; en Sprint 5 con sesión autenticada |

Ambos corren en contenedores con perfil de compose (`perf` y `security`), así que `make up` no los levanta por defecto. Los reportes de ZAP quedan en `reports/security/` (no versionados).

**Códigos de salida de ZAP:** `0` sin alertas, `1` solo advertencias, `2` hay alertas FAIL. `make` reporta error con `1` y con `2`, así que en el resultado de un sprint hay que mirar el reporte, no solo el código.

**Terminal vs. interfaz gráfica:** usamos ZAP por terminal (Docker, headless) como estándar. Es reproducible, lo mismo corre en el equipo y en CI, y deja reportes versionables. La interfaz gráfica sirve para exploración manual puntual (por ejemplo, revisar a mano un flujo con sesión), pero no es la fuente de verdad de los resultados.

**Regla de hallazgos ZAP:** todo FAIL nuevo bloquea el cierre del sprint. Los WARN se registran en el backlog con responsable.

## Sprint 1 — Contratos y base de Fase 2

**Objetivo:** contratos de pedidos, zonas y repartidores; primeros servicios; línea base de seguridad.

| Persona | Tarea | Entregable |
|---|---|---|
| Anthony | Contrato `orders.v1` (proto) y endpoints `/orders` en OpenAPI | Contrato mergeado |
| Anthony | Order Service: dominio, `created_at` vs `delivery_date`, estados, migraciones | Pruebas unitarias de dominio |
| Erick | Contrato `zones.v1` y Zone Service: zonas con PostGIS, migraciones con índice GiST | Migraciones aplicadas |
| Erick | Clasificación de coordenadas (`ST_Contains`) como caso de uso | Prueba de integración PostGIS |
| Mabe | Contrato `drivers.v1` y Driver Service: dominio y estados (`AVAILABLE`, `ASSIGNED`, `ON_ROUTE`, `OFFLINE`) | Pruebas unitarias |
| Mabe | **OWASP:** primera corrida de `make security-scan` y registro de hallazgos en el backlog | Reporte inicial archivado |
| Xabier | Web: listado y alta de pedidos sobre el contrato (mocks en Playwright) | Pantallas con pruebas Vitest |
| Xabier | Web: lint y typecheck estrictos en las nuevas pantallas | CI verde |

**Criterio de cierre:** contratos mergeados, servicios con pruebas unitarias, `make perf-smoke` en verde, hallazgos ZAP registrados.

## Sprint 2 — Cierre de Fase 2 y arranque de Fase 3

**Objetivo:** pedidos y zonas funcionando de extremo a extremo; repartidores; corregir hallazgos de seguridad.

| Persona | Tarea | Entregable |
|---|---|---|
| Anthony | Order Service expuesto por gRPC y consumido desde el Gateway | Pruebas de integración con PostgreSQL |
| Anthony | **OWASP:** corregir el WARN de `Cross-Origin-Resource-Policy` en el Gateway (detectado por ZAP) | Header agregado y prueba unitaria |
| Erick | Zone Service: CRUD de zonas y búsqueda de zona por dirección | Endpoints y pruebas de integración |
| Erick | **OWASP:** pruebas de autorización por rol (ADMIN/OPERATOR) sobre pedidos y zonas | Pruebas Go que fallan sin rol |
| Mabe | Driver Service: activar/desactivar repartidor, disponibilidad | Pruebas de integración |
| Mabe | Mobile: scaffolding de `apps/mobile` (React Native, Vertical Slices, sección 21) | App arranca con pantalla de login |
| Xabier | Web: zonas (lista y edición) y repartidores (listado) | Pruebas Vitest + Playwright |
| Xabier | Web: E2E de pedidos (crear y consultar) | Test E2E verde |

**Criterio de cierre:** Fase 2 completa; `make security-scan` sin FAIL nuevos y con el WARN de CORP corregido.

## Sprint 3 — Núcleo de Fase 4: Routing (parte 1)

**Objetivo:** planificador de rutas intercambiable, con capacidad configurable y asignación de repartidor.

| Persona | Tarea | Entregable |
|---|---|---|
| Erick | Routing Service: `Route`, `RouteStop`, interfaz `RoutePlanner` y primer algoritmo (agrupar por zona y capacidad) | Pruebas unitarias del algoritmo |
| Erick | Capacidad configurable con `MAX_ORDERS_PER_ROUTE` (sin valor fijo en código) | Prueba con distintos valores |
| Anthony | Clientes gRPC de Routing hacia Order y Zone; endpoint `POST /api/v1/route-plans/generate` en el Gateway | Prueba de integración |
| Anthony | Idempotencia base con `Idempotency-Key` en la generación de rutas | Prueba de reintento |
| Mabe | Driver Service: `ReserveDriver` y `FindAvailableDrivers` | Pruebas de integración |
| Mabe | Asignación sin conflictos con `SELECT ... FOR UPDATE` | Prueba de concurrencia |
| Xabier | Web: botón para generar rutas y vista de rutas generadas | Pruebas Vitest |
| Xabier | **OWASP:** revisar en el front que no se muestren datos que el rol no debe ver | Lista de verificación firmada |

**Criterio de cierre:** se generan rutas respetando capacidad; `make security-scan` sin FAIL nuevos.

## Sprint 4 — Fase 4 (concurrencia) y arranque de Fase 5

**Objetivo:** invariantes de asignación bajo concurrencia y pedidos de último momento.

| Persona | Tarea | Entregable |
|---|---|---|
| Erick | Pruebas de concurrencia de los invariantes (sección 16): pedido en una sola ruta activa, un repartidor por ruta | Pruebas que pasan con `-race` y bajo carga concurrente |
| Erick | Inserción de pedidos de último momento con validaciones de capacidad, fecha y zona | Pruebas del caso de uso |
| Anthony | Replanificación controlada y control optimista (`version`) en rutas | Pruebas de conflicto de versión |
| Anthony | **k6:** escenario de generación simultánea de rutas (extiende `smoke.js`) | Script k6 con umbrales |
| Mabe | Mobile: login y ruta asignada (lectura de `GET /drivers/me/route`) | Pantalla con pruebas Jest |
| Mabe | **OWASP:** configurar contexto autenticado de ZAP (JWT de un repartidor) | Contexto documentado |
| Xabier | Web: pedidos de último momento y ajustes manuales autorizados | Pruebas Vitest |

**Criterio de cierre:** invariantes de asignación cubiertos por pruebas de concurrencia; k6 de rutas dentro de umbrales en el entorno local.

## Sprint 5 — Fase 5 (cierre) y Fase 6 (seguimiento)

**Objetivo:** estados de entrega, seguimiento en vivo y pruebas de seguridad autenticadas.

| Persona | Tarea | Entregable |
|---|---|---|
| Anthony | Tracking Service: transiciones (`PICKED_UP`, `IN_TRANSIT`, `DELIVERED`) con fecha/hora y `PATCH /deliveries/{id}/status` | Pruebas de transición de estado |
| Anthony | Última ubicación en Redis (dato efímero, sección 27) | Prueba de expiración |
| Erick | Idempotencia en crear pedido y asignar ruta | Pruebas de reintento |
| Mabe | Mobile: marcar recogido, en camino y entregado; orden de paradas | Pruebas Jest |
| Xabier | Web: seguimiento en vivo por WebSocket del Gateway con fallback REST | Pruebas de reconexión |
| Mabe | **OWASP (BOLA):** un repartidor solo actualiza entregas de su ruta activa | Prueba de autorización que falla sin pertenencia |
| Erick | **OWASP:** escaneo autenticado con ZAP sobre endpoints de pedidos y rutas | Reporte sin FAIL nuevos |

**Criterio de cierre:** Fase 5 y Fase 6 completas; pruebas BOLA verdes; ZAP autenticado sin FAIL nuevos.

## Sprint 6 — Fase 7: calidad y rendimiento

**Objetivo:** datasets masivos, pruebas de volumen, carga y estrés, y cierre de seguridad.

| Persona | Tarea | Entregable |
|---|---|---|
| Anthony | Datasets de `test-data/seeds` (10 000 pedidos, 100 repartidores, 5 000 rutas históricas) | Seeds reproducibles |
| Anthony | **k6:** escenarios de volumen, carga y estrés (sección 32) con p50, p95 y p99 | Informe con métricas |
| Erick | Rendimiento de PostGIS (`ST_DWithin`, índices GiST) con el dataset masivo | Plan de consulta documentado |
| Erick | SonarQube: quality gate del proyecto | Gate aprobado |
| Mabe | **OWASP ZAP:** corrida completa autenticada y remediación de FAIL/WARN | Reporte final archivado |
| Mabe | Verificar cabeceras HTTP y rate limiting (sección 26.5) | Pruebas de cabeceras |
| Xabier | **Playwright E2E:** login, crear pedido, crear zona, generar rutas, visualizar ruta, cambio de estado | Suite E2E verde |
| Todos | Informe final de pruebas: resultados de k6, ZAP, E2E y cobertura | Documento en `docs/` |

**Criterio de cierre:** todas las fases completas; reportes de k6 y ZAP archivados; quality gate aprobado.

## Riesgos

- **Dependencias entre servicios:** Routing depende de Order, Zone y Driver. Si Sprint 2 se retrasa, Sprint 3 se reprograma y no se empieza Routing sin contratos listos.
- **Falsos positivos de ZAP:** cada hallazgo se clasifica (corregido, falso positivo justificado o aceptado con responsable). No se ignoran sin registro.
- **Escaneo activo:** ZAP envía payloads de ataque. Solo se ejecuta contra el entorno local o de pruebas.
- **Permisos en Windows:** la carpeta `reports/security/` se monta en el contenedor de ZAP; si hay problemas de escritura, revisar los permisos de Docker Desktop.
