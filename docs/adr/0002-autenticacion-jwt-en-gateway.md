# ADR 0002: El Gateway valida el JWT localmente

Estado: aceptada (Fase 1)

## Contexto

El Gateway hace la autenticación inicial y propaga la identidad (sección 8.1).
Auth Service emite los JWT (sección 8.2). Llamar a Auth en cada petición agrega
latencia y lo vuelve un punto único de falla.

## Decisión

- Auth firma tokens HS256 con información mínima: `sub`, `role`, `iss`, `iat`, `exp`.
- El Gateway verifica firma, algoritmo, emisor y expiración con el mismo
  `JWT_SECRET`, sin llamar a Auth.
- La identidad viaja a los servicios internos como metadata gRPC
  (`x-user-id`, `x-user-role`, `x-request-id`).
- Cada servicio decide la autorización sobre sus recursos (identidad + rol +
  pertenencia, sección 26.2). El Gateway no aplica reglas por rol.
- `ValidateToken` existe en el contrato de Auth para servicios que reciban
  tokens por otra vía.

## Consecuencias

- Revocar un token antes de su expiración requerirá una lista de revocación
  (Redis) consultada por el Gateway; queda para cuando se implemente la
  revocación de sesión.
- La red interna se asume privada. mTLS entre servicios es una mejora posterior.
- Si se necesita que terceros verifiquen tokens sin conocer el secreto, se
  migra a RS256/EdDSA con JWKS.
