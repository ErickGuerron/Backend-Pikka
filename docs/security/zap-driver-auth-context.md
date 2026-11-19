# Contexto autenticado de OWASP ZAP para repartidores

Sprint 4 (Mabe): configurar el contexto autenticado de ZAP con el JWT de un repartidor. Sprint 5 usa
el mismo contexto para las pruebas BOLA. Este documento describe el contexto; no contiene credenciales.

## Qué prueba

Un repartidor debe ver y tocar solo lo de su ruta activa. ZAP necesita una sesión de repartidor para
recorrer esos endpoints con autorización real y no solo como usuario anónimo.

## Cómo obtener la sesión

1. Crear un usuario de prueba con rol `DRIVER` en el entorno local. No usar datos reales.
2. Iniciar sesión por la API, no por la app:

   ```bash
   curl -s -X POST http://localhost:8080/api/v1/auth/login \
     -H 'Content-Type: application/json' \
     -d '{"email":"<correo-de-prueba>","password":"<clave-de-prueba>"}'
   ```

3. Tomar `accessToken` de la respuesta. Expira según `expiresIn`; hay que renovarlo antes de cada corrida.

Las credenciales de prueba se pasan como variables de entorno de la corrida de ZAP. No van en el repositorio
ni en el contexto exportado.

## Configurar el contexto en ZAP

- **Autenticación:** `HTTP Authorization header`, valor `Bearer <accessToken>`.
- **Usuarios:** un usuario de sesión `driver-test` y otro de sesión `customer-test` para la prueba BOLA cruzada.
- **Inclusión:** `http://localhost:8080/api/v1/.*`. Excluir `/docs` y `/health`.
- **Verificación de sesión:** una petición a `GET /api/v1/drivers/me/route` con 200 para el repartidor.

## Estado

- Las rutas del repartidor aún no existen en el Gateway (`GET /api/v1/drivers/me/route` y el resultado de parada
  siguen planeados). El contexto queda listo, pero la verificación de sesión no puede pasar hasta que existan.
- Las pruebas BOLA de pedidos dependen de la API de pedidos (PRs de Anthony).

## Referencias

- `docs/planning/division-de-tareas.md`, Sprint 4 y Sprint 5.
- ADR 0007, secciones 4 y 5: registro de entrega y penalización.
