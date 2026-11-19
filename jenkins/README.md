# Jenkins para Pikka (APE 7)

Jenkins en Docker que ejecuta el `Jenkinsfile` de la raiz: pruebas unitarias con cobertura, despliegue con Docker Compose
(local o en una VM por SSH), frontend en Nginx y prueba de humo.

## Levantar Jenkins (cada integrante)
1. Requisito: Docker Desktop encendido.
2. Desde esta carpeta: `docker compose up -d --build`
3. Contrasena inicial: `docker exec pikka-ci cat /var/jenkins_home/secrets/initialAdminPassword`
4. Abrir http://localhost:9090, elegir "Install suggested plugins" y crear el usuario administrador.
   Los plugins del pipeline (Pipeline, Git, SSH Agent, Credentials Binding, Coverage) ya vienen en la imagen.

## Credenciales (Manage Jenkins > Credentials > System > Global)
| ID | Tipo | Contenido |
|----|------|-----------|
| `pikka-env` | Secret file | `.env` del backend para local |
| `pikka-env-nube` | Secret file | `.env` del backend para la nube |
| `pikka-env-servidor` | Secret file | `.env` para la VM Linux dedicada (CORS con la IP de la VM) |
| `servidor-ssh` | SSH Username with private key | Llave privada para entrar por SSH a la VM dedicada (TARGET = servidor) |
| `oracle-ssh` | SSH Username with private key | Llave del servidor en la nube (solo TARGET = nube) |

El `.env` no se sube al repositorio: cada integrante crea el suyo con sus propios valores.

## Crear el job
New Item > `pikka-backend` > Pipeline > "Pipeline script from SCM" > Git >
`https://github.com/ErickGuerron/Backend-Pikka.git`, rama `*/feat/jenkins-ci` (o `*/develop` tras el merge), Script Path `Jenkinsfile`.

## Ejecutar
Build with Parameters:
- `TARGET = local`: despliega en el Docker de este equipo (web en 5173, app móvil en 8082).
- `TARGET = servidor`: despliega por SSH en una VM Linux dedicada (`SERVER_HOST = usuario@IP`); web en `http://IP` (puerto 80) y app móvil en `http://IP:8082`.
- `TARGET = nube`: servidor de Google Cloud con Cloudflare Tunnel.
`FRONTEND_BRANCH = develop` por defecto.
La cobertura queda en el menu del build (Coverage Report) y los archivos en Artifacts.

## Que vuelva solo tras apagar el equipo
- Jenkins y todos los contenedores del backend y del frontend usan `restart: unless-stopped`.
- Activar en Docker Desktop: Settings > General > "Start Docker Desktop when you sign in to your computer".
- Al iniciar sesion en Windows, Docker Desktop arranca y los contenedores vuelven solos; los datos de PostgreSQL y de Jenkins
  persisten en volumenes. No ejecutar `docker compose down -v` ni `docker stop`, porque una parada manual impide el reinicio automatico.
