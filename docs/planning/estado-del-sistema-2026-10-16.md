# Estado del sistema — 2026-10-16

Foto tomada al cierre de la sesión de trabajo del 16 de octubre de 2026. Cada afirmación indica cómo se comprobó. Los responsables salen de `docs/planning/division-de-tareas.md` (backend y frontend); donde el plan no asigna a nadie, aparece como **(propuesto)**.

Equipo: **Anthony** (Backend), **Erick** (Full stack), **Mabe** (Full stack), **Xabier** (Frontend).

## 1. Resumen

- **El cliente ya puede registrarse y hacer un pedido de punta a punta por API**, con teléfono, ciudad, referencia del lugar y teléfono de contacto. Probado contra el stack docker reconstruido.
- **La web del cliente también lo hace** (registro y pedido con confirmación y zonas en el mapa), probado en el navegador contra ese backend. Esos cambios están en commits locales; **otro agente se encarga del front** (ver sección 4).
- **Lo que sigue sin existir** son las rutas del repartidor en el gateway (devuelven 501), y con eso toda la app móvil del repartidor funciona solo con datos de demostración.

## 2. Hecho

### Backend (PR abierto: [Backend-Pikka#41](https://github.com/ErickGuerron/Backend-Pikka/pull/41), base `develop`)
| Qué | Estado | Cómo se comprobó |
|---|---|---|
| Registro de cliente con `phone` y `city` obligatorios (validados y normalizados en el gateway) | Hecho | E2E: sin teléfono o con teléfono inválido 400; válido 201 |
| Pedido con `addressReference` y `contactPhone` obligatorios y `postalCode` opcional, guardados y devueltos en crear, mis pedidos, excepciones y revisión | Hecho | E2E: sin referencia 400, teléfono inválido 400, completo 201 con los campos |
| La corrección de un pedido en revisión conserva los tres campos | Hecho | Tests unitarios del caso de uso |
| `GET /api/v1/coverage/zones` para cualquier rol autenticado (zonas activas, solo `id`, `code`, `name`, `ring`, `holes`) | Hecho | E2E: cliente 200, sin token 401 |
| `GET /api/v1/zones` sigue prohibido al cliente | Hecho | E2E: 403 |
| Migración `000012` (`TEXT NOT NULL DEFAULT ''`) | Aplicada al arrancar order-service en el stack local | El pedido nuevo se guardó y se leyó con los campos |
| Contrato: protos, código generado, OpenAPI, ADR 0008 | Hecho | `buf lint` y `buf generate` sin errores |
| `go build`, `go vet` y `go test ./...` en order-service, zone-service y api-gateway | Pasan | Ejecutados y repetidos al revisar |

### Backend, otro PR abierto
- [Backend-Pikka#40](https://github.com/ErickGuerron/Backend-Pikka/pull/40): el gateway deja de enviar `"stockQuantity": null` y lo omite, como dice el contrato. Con el campo en `null`, el front mostraba "Ocurrió un error inesperado" en la pantalla de productos.

### Ya existía y se comprobó
- Registro, login, catálogo público, crear pedido con productos, listar mis pedidos y cancelar: funcionan por API (E2E de esta sesión).
- Catálogo, stock y productos (ADR 0009), cancelación de pedidos y estado del cliente: ya estaban en `develop` (PRs #28–#33 según el historial).
- Driver Service completo (dominio, estados, activar y desactivar, disponibilidad, reservar, buscar, registro de entrega, verificación de llegada por GPS) con tests; solo alcanzable por gRPC. Auditado por lectura de código, **no se ejecutó su suite**.

### Frontend (commits locales, sin push; ver sección 4)
- Web: selector de productos al crear pedido (`items`), registro con ciudad y teléfono, pedido con referencia, código postal y teléfono, paso de confirmación, zonas de cobertura dibujadas en el mapa con aviso de dentro o fuera, paginación de 5 filas en las tablas, KPIs compartidos, validaciones del modal de producto, ajuste de stock como modal, carrusel de la landing con datos reales. Comprobado con 464 tests de Vitest, `tsc` y eslint sin errores, y recorrido real en el navegador.
- Móvil: lo mismo para el cliente, excepto las zonas. Comprobado con 114 tests de Jest y `tsc`.

## 3. Pendiente y quién debe hacerlo

### Bloquea al repartidor
| Qué | Responsable | Detalle |
|---|---|---|
| Rutas reales en el gateway: `GET /drivers/me/route`, estado de parada y resultado de parada | **Anthony** (el plan le asigna las rutas del gateway); Mabe dejó los stubs 501 a propósito | Hoy las tres devuelven 501 (`router.go`, `driverRouteNotImplemented`). Deben llamar a driver-service, routing y order. |
| Sacar las pantallas móviles del repartidor de `demo.ts` | **Mabe** | Depende de la fila anterior. |
| Pruebas BOLA: el repartidor solo toca su ruta activa | **Mabe y Erick** | Faltan los handlers, así que no hay qué probar. |

### Seguridad y pruebas
| Qué | Responsable | Detalle |
|---|---|---|
| ZAP autenticado completo (cliente y repartidor) con informe final y remediación | **Mabe** | Solo existen el baseline y el documento del contexto de repartidor. |
| BOLA del cliente más allá de la cancelación | **Mabe y Erick** | Hoy solo está cubierta la cancelación. |
| Correr los tests de integración (etiqueta `integration`) en CI con PostgreSQL | **Erick** (propuesto) | Compilan, pero no se ejecutaron en esta sesión por falta de base de datos de pruebas. |
| Decidir si la asignación sin conflictos usa `FOR UPDATE` o se corrige el plan a "bloqueo optimista por `version`" | **Erick y Mabe** (propuesto) | El código usa `version` y el plan dice `FOR UPDATE`. |

### Producto
| Qué | Responsable | Detalle |
|---|---|---|
| Decidir qué hacer con clientes y pedidos anteriores a la migración, que quedan con teléfono, ciudad y referencia vacíos | **Anthony** con el equipo | Hoy no se bloquean; solo faltan los datos. |
| Devolver los productos de cada pedido en `GET /orders/mine` | **Anthony** (propuesto) | "Mis pedidos" no puede mostrarlos: la lista no incluye `items`. |
| Notas del pedido | **Anthony** (propuesto) | El contrato no tiene el campo; el front las ignora. |
| Precargar teléfono y ciudad del perfil en el pedido | **Anthony** (propuesto) | Falta un endpoint de perfil del cliente. |

### Móvil
| Qué | Responsable | Detalle |
|---|---|---|
| Capa de zonas y aviso de cobertura | **Mabe** | La app no tiene componente de mapa ni dependencia de mapas. |
| Exigir URL `https` en builds de producción | **Mabe** | Hoy es solo una verificación manual del pipeline. |
| Configurar eslint en `apps/mobile` | **Mabe** (propuesto) | No hay config; `npx expo lint` falla y, al correrlo, instala dependencias y modifica `package.json`. |
| Probar en dispositivo o emulador | **Mabe** | No se probó el diseño visual. |

### Operación
| Qué | Responsable | Detalle |
|---|---|---|
| Revisar y mergear los PR #40 y #41 | **Erick** (propuesto, revisor) | Aún sin checks de CI reportados. |
| Reiniciar el stack tras el merge | Quien despliegue | Para que el gateway use el contrato nuevo hay que reconstruir `order-service`, `zone-service` y `api-gateway`. |

## 4. Frontend: lo hace otro agente

Quedan en la rama local `feat/order-form-items-contact` (4 commits, sin push, sin PR):
`e427197` items al crear pedido, `1a55888` registro con ciudad y teléfono, `98b459f` contacto, confirmación y zonas, `3aaa3cf` móvil.

**Dependencia de orden:** el front nuevo exige los campos que añade el PR #41. Si se mergea el backend sin el front, el registro y el pedido de la web actual de `develop` darán 400. Si se mergea el front sin el backend, el registro y el pedido darán 400 al revés. Deben salir juntos.

## 5. No verificado

- Ninguna prueba en dispositivo móvil ni en emulador.
- No se ejecutó la suite de Driver Service, ni sus tests de integración.
- No se auditó el estado de tracking ni de rutas del sprint 5 en adelante.
- La migración `000012` se aplicó en el stack local, pero nunca contra una base limpia ni en CI.
- El estado de Driver Service y de las tareas de Mabe sale de la lectura del código por un agente, no de ejecutarlo.

## 6. Residuos de pruebas en la base local

Quedaron cuentas de prueba con correos `*@example.test` (por ejemplo `e2e.…`, `ui.…`, `signup.ui2…`, `ui.contacto.final…`) y varios pedidos de prueba, algunos cancelados. Conviene limpiarlos antes de una demostración.
