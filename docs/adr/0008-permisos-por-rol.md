# ADR 0008: Permisos por rol en la web y en la API

Estado: aceptada

Complementa al ADR 0002 (cada servicio decide la autorización) y al ADR 0007
(app móvil única para clientes y repartidores).

## Contexto

Hoy la web muestra a todos los roles las mismas secciones, y un repartidor puede
ver el listado de repartidores y el seguimiento operativo. Eso no corresponde a
su función. Además, un endpoint que devuelve datos a quien no debe recibirlos es
un fallo de seguridad aunque la pantalla esté oculta.

## Decisión

### 1. Matriz de acceso a secciones de la web

| Sección | ADMIN | OPERATOR | DRIVER |
|---|---|---|---|
| Inicio | Completo | Completo | Sin acceso web |
| Pedidos: crear y consultar | Completo | Completo | Sin acceso web |
| Cola de excepciones y disputas | Completo | Completo | Sin acceso web |
| Zonas | Completo | Solo lectura | Sin acceso web |
| Rutas: generar y consultar | Completo | Completo | Sin acceso web |
| Repartidores: listado y disponibilidad | Completo | Solo lectura | Sin acceso web |
| Seguimiento operativo (estado de todas las entregas) | Completo | Solo lectura | Sin acceso web |
| Usuarios | Completo | Sin acceso | Sin acceso web |

- **DRIVER no usa la web.** Su trabajo está en la app móvil (ADR 0007). Si entra
  a la web, ve una pantalla que lo indica, sin datos.
- **OPERATOR no gestiona usuarios.** El alta y la consulta de usuarios quedan en
  ADMIN, salvo la consulta de uno mismo.

### 2. Qué ve cada rol en la app móvil

| Vista | CUSTOMER | DRIVER |
|---|---|---|
| Sus pedidos y su estado | Completo (solo los propios) | Sin acceso |
| Ruta del pedido propio: posición de la parada | Solo lectura (solo la parada propia) | Sin acceso |
| Cobertura: zonas activas (solo contorno, para dibujarlas en el mapa) | Solo lectura | Solo lectura |
| Su ruta asignada y el orden de paradas | Sin acceso | Completo (solo la ruta activa) |
| Registro de la entrega de cada parada | Sin acceso | Completo (solo sus paradas) |
| Listado de repartidores u otros repartidores | Sin acceso | Sin acceso |

Un repartidor nunca ve la ruta completa de otros repartidores ni el seguimiento
de todas las entregas. Un cliente nunca ve la ruta completa, solo la posición de
su propio pedido.

### 3. Regla de la API: rechazar antes de consultar

Cada endpoint aplica en este orden:

1. **Autenticación**: JWT válido (ADR 0002).
2. **Rol**: si el rol no está en la matriz para ese endpoint, responde `403` sin
   consultar la base de datos.
3. **Alcance**: si el rol está permitido, la consulta filtra por el alcance del
   rol (propietario, ruta activa, zona). Nunca se trae todo y se filtra después.

Un `404` se usa cuando el recurso existe pero pertenece a otro alcance, para no
revelar que existe.

### 4. Estado de implementación

| Endpoint | Regla | Estado |
|---|---|---|
| `POST /api/v1/users` | Solo `ADMIN` | Implementado y probado en Auth Service |
| `GET /api/v1/users/{id}` | `ADMIN`, `OPERATOR` o el propio usuario | Implementado y probado en Auth Service |
| `GET /api/v1/users` (listado) | Solo `ADMIN` | Falta en el contrato OpenAPI y en Auth Service |
| `GET /api/v1/coverage/zones` | Cualquier rol autenticado (`CUSTOMER` incluido): solo zonas `ACTIVE`, con `id`, `code`, `name`, `ring` y `holes`; sin depósito, capacidad ni configuración | Implementado y probado en Zone Service (`ListCoverageZones`) y en el Gateway |
| `GET /api/v1/zones` (listado completo) | Solo `ADMIN` y `OPERATOR`; `CUSTOMER` recibe `403` | Sin cambios: la cobertura usa un endpoint aparte para no abrir la configuración |
| Pedidos, zonas, repartidores, rutas, seguimiento | Según la matriz | Pendientes: se implementan con su servicio y deben probar su rol |

### 5. Frontend

La web aplica la misma matriz para ocultar secciones, proteger rutas y no
lanzar consultas de secciones que el rol no ve. Esto es solo experiencia de uso.
La autorización real siempre está en la API. Si una sección no permitida se
abre por URL, el usuario vuelve a Inicio con un aviso, o ve la pantalla de
repartidor si su rol no tiene web.

## Consecuencias

- Cada servicio nuevo incluye pruebas de autorización por rol antes de cerrar su
  sprint: un rol sin permiso recibe `403` y la consulta no se ejecuta.
- El Gateway no cambia (ADR 0002).
- El frontend tiene un único mapa de permisos que refleja esta matriz.
- Pendiente: agregar `GET /api/v1/users` (solo `ADMIN`) al contrato, porque la
  pantalla de usuarios lo necesita.
