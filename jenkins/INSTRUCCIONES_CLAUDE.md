# Guía para Claude: reproducir el flujo de Jenkins de Pikka (APE 7)

Este documento es para el asistente (Claude) de un integrante del grupo que quiere levantar el mismo flujo de
despliegue automatizado que ya funciona en el equipo de María Zapata. Léelo completo antes de ejecutar nada.

## Qué se construyó
- Jenkins corre **dentro de un contenedor Docker** (`pikka-ci`, panel en el puerto 9090) y usa el Docker del equipo
  mediante `/var/run/docker.sock`.
- El `Jenkinsfile` de la raíz de este repo define el pipeline: Checkout → Preparar entorno (copia el `.env` desde una
  credencial) → Pruebas unitarias y cobertura (siete servicios Go, dentro de `docker build`) → Acceso a la nube
  (solo `TARGET=nube`) → Desplegar backend (`docker compose -f docker-compose.yml -f docker-compose.ci.yml up -d --build`)
  → Frontend web (Nginx, repo Frontend-Pikka) → Prueba de humo (`/ready`).
- `docker-compose.ci.yml` reemplaza los montajes de archivos locales (init de PostgreSQL y `redis.conf`) por imágenes que los
  llevan incluidos, porque desde un Jenkins en contenedor o un Docker remoto esas rutas no existen.
- Parámetros: `TARGET` (`local`, `servidor` o `nube`), `FRONTEND_BRANCH`, `SERVER_HOST` (`usuario@IP` de una VM Linux dedicada, solo servidor) y `CLOUD_HOST` (solo nube).
- `TARGET=servidor` usa el Docker de la VM por SSH (`DOCKER_HOST=ssh://...`): web en el puerto 80, app móvil en el 8082. La VM necesita Docker Engine, OpenSSH y el usuario en el grupo `docker`.
- Este flujo automatiza el despliegue manual del APE 6 (docker compose + docker build/run a mano).

## Requisitos
Docker Desktop encendido (Docker Engine y Compose), Git y acceso a Internet. Rama de trabajo: `feat/jenkins-ci`
(o `develop` si ya se hizo el merge). El frontend usa la rama `feat/docker-frontend` de Frontend-Pikka (o `develop` tras el merge).

## Pasos
1. Clonar el repo y entrar a la rama: `git clone https://github.com/ErickGuerron/Backend-Pikka.git` y `git checkout feat/jenkins-ci`.
2. Crear el archivo `.env` del backend a partir de `.env.example` (si existe) o del `.env` que el usuario tenga. **Nunca**
   escribir, mostrar ni subir secretos al repositorio.
3. `cd jenkins && docker compose up -d --build` (construye Jenkins con sus plugins: Pipeline, Git, SSH Agent, Credentials
   Binding, Coverage).
4. Obtener la contraseña inicial: `docker exec pikka-ci cat /var/jenkins_home/secrets/initialAdminPassword`; abrir
   http://localhost:9090, instalar los plugins sugeridos y crear el usuario administrador (lo hace el usuario en el navegador).
5. Crear credenciales en Manage Jenkins > Credentials > System > Global (también lo hace el usuario):
   - `pikka-env`: tipo *Secret file*, con su `.env` local.
   - `pikka-env-servidor`: *Secret file* con el `.env` para la VM dedicada (CORS con la IP de la VM).
   - `servidor-ssh`: *SSH Username with private key* para entrar a la VM (la llave pública va en `~/.ssh/authorized_keys` de la VM). Passphrase vacía si la llave no la tiene.
   - `pikka-env-nube` y `oracle-ssh`: solo si va a desplegar en la nube.
6. Crear el job `pikka-backend`: Pipeline > *Pipeline script from SCM* > Git > URL del repo > rama `*/feat/jenkins-ci` >
   Script Path `Jenkinsfile`.
7. Ejecutar *Build with Parameters* con `TARGET=local` (o `servidor` con `SERVER_HOST`). Verificar: `docker ps` (nueve contenedores del backend y `pikka-web`),
   http://localhost:5173 y, en el build, el reporte *Coverage Report* y la línea `COBERTURA <servicio>: total ...%` por servicio.

## Reinicio automático (demostración)
Jenkins y los contenedores usan `restart: unless-stopped`. En Docker Desktop activar *Start Docker Desktop when you sign in*.
Tras reiniciar el equipo e iniciar sesión, todo vuelve solo. No usar `docker stop`, `docker compose down` ni `down -v`
antes de apagar. Si algo no volvió, comprobar con `docker ps -a` y `docker logs <contenedor>`.

## Problemas ya encontrados y su solución
| Síntoma | Causa | Solución |
|---|---|---|
| `[setup failed]` en routing-service | su `go.mod` no declara testify y las pruebas corrían con `GOWORK=off` | ejecutar las pruebas en modo workspace (no usar `GOWORK=off`) |
| `FROM` vacío al construir PostgreSQL | Compose interpola `${PG_IMAGE}` dentro de `dockerfile_inline` | escribir `$${PG_IMAGE}` |
| `docker buildx` desconocido | faltaba el plugin en la imagen de Jenkins | `docker-buildx-plugin` ya está en el Dockerfile |
| `proxy.golang.org ... unexpected EOF` | corte de red al bajar módulos de Go | el pipeline reintenta 3 veces y usa `GOPROXY=https://proxy.golang.org,direct` |
| El build pasa pero no hay contenedores en la nube | se ejecutó con `TARGET=local` | repetir con `TARGET=nube` y `CLOUD_HOST` |
| Puerto 9090 u otro ocupado | otro servicio en el equipo | cambiar el puerto izquierdo en `jenkins/docker-compose.yml` |

## Reglas para Claude
- Ejecuta los comandos por partes y pide al usuario lo que requiera el navegador o sus secretos.
- No publiques ni imprimas contenido de `.env`, llaves SSH ni contraseñas.
- No hagas `push`, merge ni borres volúmenes sin que el usuario lo pida.
- Si un build falla, pide el log de consola completo y busca primero la tabla de problemas de arriba.
