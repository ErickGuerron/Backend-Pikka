# Reporte inicial de seguridad: OWASP ZAP (2026-10-09)

Primera corrida de `make security-scan` (equivalente: `docker compose --profile security run --rm zap`).
Los reportes completos (`zap-report.html`, `.json`, `.md`) quedan en `reports/security/`, que no se versiona; este documento archiva el resumen y los hallazgos.

## Condiciones de la corrida

| Dato | Valor |
|------|-------|
| Fecha | 2026-10-09 |
| Commit | `af20abd` (`develop`) |
| Objetivo | `http://api-gateway:8080/openapi.yaml` (red interna de compose, entorno local) |
| Tipo | Escaneo activo de API (`zap-api-scan.py -f openapi -m 5`) |
| Imagen | `ghcr.io/zaproxy/zaproxy:stable` (`sha256:7aaa659b0d43078febd82e29bad112285c370727e86ab8340444220e17d9f0d2`) |
| Sesión | Sin autenticar |
| Servicios arriba | postgres, redis, auth-service, zone-service, api-gateway |

## Resultado

`FAIL-NEW: 0`, `WARN-NEW: 1`, `PASS: 118`. Código de salida de ZAP: 2 (indica advertencias; el comentario de `docker-compose.yml` lo describe al revés y conviene corregirlo).

| Riesgo | Alertas |
|--------|---------|
| Alto | 0 |
| Medio | 0 |
| Bajo | 1 |
| Informativo | 4 |

## Hallazgos para el backlog

| ID | Hallazgo | Riesgo | Instancias | Acción | Responsable previsto |
|----|----------|--------|------------|--------|----------------------|
| ZAP-90004 | `Cross-Origin-Resource-Policy` ausente o inválido en `/health`, `/ready` y `/openapi.yaml` | Bajo | 3 | Agregar el header en el Gateway y una prueba unitaria | Anthony (ya asignado en `division-de-tareas.md`) |
| ZAP-INFO-1 | Respuestas 4xx devueltas por el servidor (39 instancias) | Informativo | 39 | Esperado: la corrida no autenticada recibe 401 en endpoints protegidos | Ninguna |

## Limitaciones de esta línea base

- El 99 % de las respuestas fueron 4xx porque la corrida no lleva sesión. Los endpoints protegidos solo se probaron hasta la barrera de autenticación; la lógica de negocio detrás de ella no se escaneó.
- La cobertura real llega con la corrida autenticada prevista para el Sprint 5 y con las pruebas de autorización por rol (ADMIN/OPERATOR).
- El Driver Service todavía no expone transporte, así que no forma parte del objetivo.
- Es un escaneo activo: solo se ejecuta contra el entorno local o de pruebas, nunca contra producción.

## Cómo repetirla

```bash
docker compose up -d --build
docker compose --profile security run --rm zap
```
