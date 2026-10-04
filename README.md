# Backend-Pikka

Backend del sistema Pikka. El frontend vive en
[Frontend-Pikka](https://github.com/ErickGuerron/Frontend-Pikka).

Sistema de gestión de entregas y rutas de última milla: pedidos, zonas, armado
de rutas, repartidores y seguimiento. La base técnica completa está en
[`docs/architecture/BASE_TECNICA.md`](docs/architecture/BASE_TECNICA.md).

## Estado

| Fase | Contenido | Estado |
|---|---|---|
| 1. Fundaciones | Monorepo, Docker, PostgreSQL+PostGIS, Redis, API Gateway, Auth Service, contratos gRPC y OpenAPI, CI, SonarQube | Hecha |
| 2. Pedidos y zonas | Order Service, Zone Service, clasificación geográfica | Pendiente |
| 3. Repartidores | Driver Service, disponibilidad, app móvil base | Pendiente |
| 4. Rutas | Routing Service, algoritmo, capacidad, asignación concurrente | Pendiente |
| 5. Último momento | Inserción de pedidos tardíos, replanificación, idempotencia | Pendiente |
| 6. Seguimiento | Tracking Service, sincronización web, WebSocket | Pendiente |
| 7. Calidad y rendimiento | k6, OWASP ZAP, pruebas de volumen/carga/estrés | Pendiente |

## Arranque rápido

Requisitos: Docker con Compose, Go 1.26 y `make`.

```bash
cp .env.example .env      # cambia las contraseñas y JWT_SECRET
make up                   # postgres, redis, auth-service, api-gateway
curl localhost:8080/ready
```

Iniciar sesión con el administrador inicial (definido en `.env`):

```bash
curl -s -X POST localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@lastmile.local.dev","password":"change_me_admin_password"}'
```

Crear un repartidor con el token obtenido:

```bash
curl -s -X POST localhost:8080/api/v1/users \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"email":"driver1@example.com","password":"password123","fullName":"Repartidor Uno","role":"DRIVER"}'
```

## Comandos

```text
make test               pruebas unitarias (go test -race)
make test-integration   pruebas contra el PostgreSQL de docker compose
make lint               go vet + golangci-lint
make proto              regenera el código de los contratos gRPC
make openapi-lint       valida contracts/openapi/openapi.yaml
make logs / make down
```

## Estructura

```text
services/           un módulo Go por microservicio, arquitectura hexagonal
  api-gateway/      REST /api/v1, JWT, CORS, rate limit, errores, gRPC hacia adentro
  auth-service/     usuarios, roles, credenciales, emisión de JWT (esquema auth)
contracts/
  proto/            contratos gRPC versionados (auth.v1, ...), generados en gen/go
  openapi/          contrato de la API pública
infrastructure/     init de PostgreSQL (esquemas y usuarios por servicio), Redis
docs/               arquitectura, algoritmos y ADRs
```

## Decisiones de la Fase 1

Ver [`docs/adr`](docs/adr). En resumen:

- Cada servicio es un módulo Go independiente con su propio `go.mod`, `Dockerfile`
  y migraciones embebidas que se aplican al arrancar. `go.work` une los módulos
  para desarrollo local.
- Una sola instancia de PostgreSQL, un esquema y un usuario por servicio. El
  usuario de cada servicio no puede leer ni escribir en los esquemas de los
  demás (verificado: `permission denied for schema zones`).
- El Gateway valida el JWT localmente (HS256, secreto compartido con Auth) y
  propaga `x-user-id`, `x-user-role` y `x-request-id` como metadata gRPC. La
  autorización sobre recursos la decide cada servicio; el Gateway no tiene
  reglas de negocio.
- Toda llamada gRPC sale con deadline (`GRPC_TIMEOUT`, 3 s por defecto).
- Los errores de cualquier servicio llegan al cliente con un formato único
  `{"error":{"code","message","requestId"}}`.

## Notas de dependencias

- `google.golang.org/grpc` está fijado a un commit de `master`
  (`v1.85.0-dev.0.20260825072537-93e31b48545e`) porque la corrección de
  GO-2026-6443 (pánico del servidor por cabeceras `:authority`/`Host`
  ausentes) aún no está en una versión publicada. Al salir `v1.85.0`, se
  cambia a esa versión.
