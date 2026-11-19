pipeline {
    agent any
    options { timestamps(); disableConcurrentBuilds() }

    parameters {
        choice(name: 'TARGET', choices: ['local', 'nube'], description: 'Entorno de despliegue')
        string(name: 'FRONTEND_BRANCH', defaultValue: 'develop', description: 'Rama de Frontend-Pikka que se construye')
        string(name: 'CLOUD_HOST', defaultValue: 'anthony@34.73.92.242', description: 'Solo nube: usuario@IP del servidor')
        string(name: 'PUBLIC_WEB', defaultValue: 'https://pikka.factuproec.com', description: 'Solo nube: URL pública de la web')
        string(name: 'PUBLIC_MOBILE', defaultValue: 'https://app-pikka.factuproec.com', description: 'Solo nube: URL pública de la app móvil')
        string(name: 'PUBLIC_API', defaultValue: 'https://api-pikka.factuproec.com', description: 'Solo nube: URL pública de la API')
    }

    environment {
        COMPOSE  = 'docker compose -f docker-compose.yml -f docker-compose.ci.yml'
        NETWORK  = 'last-mile-logistics_default'
        SERVICES = 'auth-service zone-service order-service driver-service tracking-service routing-service api-gateway'
    }

    stages {
        stage('Checkout') {
            steps { checkout scm }
        }

        stage('Preparar entorno') {
            steps {
                script {
                    env.ENV_CRED = params.TARGET == 'nube' ? 'pikka-env-nube' : 'pikka-env'
                    env.DH       = params.TARGET == 'nube' ? "ssh://${params.CLOUD_HOST}" : ''
                    // En la nube el servidor se comparte con otro sistema que ocupa 80/443: la web y la app móvil no
                    // publican puertos; Internet entra por Cloudflare Tunnel (contenedor pikka-tunnel) a la red interna.
                    env.WEB_PORTS  = params.TARGET == 'nube' ? '' : '-p 5173:80'
                    env.MOB_PORTS  = params.TARGET == 'nube' ? '' : '-p 8082:80'
                    env.MOBILE_API = params.TARGET == 'nube' ? params.PUBLIC_MOBILE : 'http://localhost:8082'
                }
                withCredentials([file(credentialsId: env.ENV_CRED, variable: 'ENV_FILE')]) {
                    sh 'cp "$ENV_FILE" .env'
                }
            }
        }

        stage('Pruebas unitarias y cobertura') {
            steps {
                // Las pruebas corren dentro de un docker build: el contexto lo envia el cliente, sin montar carpetas del equipo.
                // La etapa "cov" solo contiene los informes de cobertura, que se exportan a reports/coverage/<servicio>.
                writeFile file: 'Dockerfile.test', text: '''FROM golang:1.26-alpine AS test
ENV CGO_ENABLED=0 GOPROXY=https://proxy.golang.org,direct
RUN go install github.com/boumenot/gocover-cobertura@latest
WORKDIR /src
COPY . .
ARG SERVICE
RUN cd services/${SERVICE} \\
 && go test -covermode=atomic -coverprofile=/cov.out ./... \\
 && echo "COBERTURA ${SERVICE}: $(go tool cover -func=/cov.out | tail -1)" \\
 && gocover-cobertura < /cov.out > /coverage.xml

FROM scratch AS cov
COPY --from=test /cov.out /coverage.out
COPY --from=test /coverage.xml /coverage.xml
'''
                sh '''
                rm -rf reports/coverage; mkdir -p reports/coverage
                for s in auth-service api-gateway order-service zone-service driver-service routing-service tracking-service; do
                    ok=0
                    for intento in 1 2 3; do
                        if docker build --progress=plain -f Dockerfile.test --target cov --build-arg SERVICE=$s --output type=local,dest=reports/coverage/$s .; then ok=1; break; fi
                        echo "Intento $intento fallido para $s (posible corte de red al descargar modulos de Go); reintentando..."
                        sleep 5
                    done
                    [ "$ok" = 1 ] || exit 1
                done
                '''
            }
            post {
                always {
                    archiveArtifacts artifacts: 'reports/coverage/**', allowEmptyArchive: true
                    recordCoverage(
                        tools: [[parser: 'COBERTURA', pattern: 'reports/coverage/**/coverage.xml']],
                        sourceCodeRetention: 'NEVER'
                    )
                }
            }
        }

        stage('Acceso a la nube') {
            when { expression { params.TARGET == 'nube' } }
            steps {
                withCredentials([sshUserPrivateKey(credentialsId: 'oracle-ssh', keyFileVariable: 'KEY')]) {
                    sh '''
                    mkdir -p ~/.ssh
                    cp "$KEY" ~/.ssh/pikka_oracle && chmod 600 ~/.ssh/pikka_oracle
                    printf "Host *\\n  IdentityFile ~/.ssh/pikka_oracle\\n  StrictHostKeyChecking accept-new\\n  ServerAliveInterval 15\\n  ServerAliveCountMax 8\\n" > ~/.ssh/config
                    '''
                }
            }
        }

        stage('Archivos de la app móvil') {
            steps {
                // Imagen de la app móvil (Expo exportada a web + Nginx). El contexto es apps/ porque importa apps/shared.
                writeFile file: 'cd/mobile-web.Dockerfile', text: '''FROM node:22-alpine AS build
WORKDIR /app/mobile
COPY mobile/package.json mobile/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY mobile/ ./
COPY shared/ /app/shared/
ARG EXPO_PUBLIC_API_URL
ENV EXPO_PUBLIC_API_URL=${EXPO_PUBLIC_API_URL}
RUN rm -f .env && npx expo export --platform web --output-dir dist

FROM nginx:1.27-alpine
COPY web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/mobile/dist /usr/share/nginx/html
HEALTHCHECK --interval=10s --timeout=3s --retries=5 CMD wget -qO- http://127.0.0.1/ >/dev/null || exit 1
'''
                // apps/.dockerignore excluye mobile/; esta imagen usa su propio ignorado.
                writeFile file: 'cd/mobile-web.Dockerfile.dockerignore', text: '''**/node_modules
**/dist
**/coverage
**/.env
**/.env.local
**/.expo
'''
            }
        }

        stage('Desplegar en local') {
            when { expression { params.TARGET == 'local' } }
            steps {
                sh '$COMPOSE up -d --build'
                dir('frontend') {
                    git url: 'https://github.com/ErickGuerron/Frontend-Pikka.git', branch: params.FRONTEND_BRANCH
                }
                sh 'cp cd/mobile-web.Dockerfile cd/mobile-web.Dockerfile.dockerignore frontend/apps/'
                dir('frontend/apps') {
                    sh '''
                    docker build -f web/Dockerfile -t pikka/web:1.0 .
                    docker build -f mobile-web.Dockerfile --build-arg EXPO_PUBLIC_API_URL="$MOBILE_API" -t pikka/mobile-web:1.0 .
                    docker rm -f pikka-web pikka-mobile >/dev/null 2>&1 || true
                    docker run -d --name pikka-web    --restart unless-stopped --network $NETWORK $WEB_PORTS pikka/web:1.0
                    docker run -d --name pikka-mobile --restart unless-stopped --network $NETWORK $MOB_PORTS pikka/mobile-web:1.0
                    '''
                }
            }
        }

        stage('Desplegar en la nube') {
            when { expression { params.TARGET == 'nube' } }
            steps {
                // La compilación se ejecuta EN el servidor, en segundo plano (setsid nohup): un corte de la conexión
                // SSH ya no la interrumpe. Jenkins solo actualiza los repositorios del servidor al commit que se
                // despliega, copia el .env y el script, lanza el despliegue y sigue su log hasta que termina.
                writeFile file: 'cd/deploy-nube.sh', text: '''#!/bin/sh
# Despliegue de Pikka en el servidor. Lo lanza Jenkins; escribe su código de salida en pikka-cd.status.
cd "$HOME/pikka_gestion" || exit 1
trap 'echo $? > "$HOME/pikka_gestion/pikka-cd.status"' EXIT
set -e
COMPOSE="docker compose --progress quiet -f docker-compose.yml -f docker-compose.ci.yml"
cd Backend-Pikka
echo "Backend $(git log --oneline -1)"
# Una imagen a la vez: el servidor (2 vCPU) atiende otro sistema en producción.
for s in $SERVICES postgres redis; do
    inicio=$(date +%s); echo "== build $s"
    $COMPOSE build $s
    echo "   listo en $(( $(date +%s) - inicio )) s"
done
$COMPOSE up -d
$COMPOSE ps --format "table {{.Service}}\\t{{.Status}}\\t{{.Ports}}"
cd ../Frontend-Pikka/apps
echo "Frontend $(git log --oneline -1)"
cp ../../mobile-web.Dockerfile ../../mobile-web.Dockerfile.dockerignore .
echo "== build web"; docker build -q -f web/Dockerfile -t pikka/web:1.0 .
echo "== build app movil"; docker build -q -f mobile-web.Dockerfile --build-arg EXPO_PUBLIC_API_URL="$MOBILE_API" -t pikka/mobile-web:1.0 .
docker rm -f pikka-web pikka-mobile >/dev/null 2>&1 || true
# Sin puertos publicados: el servidor ya tiene 80/443 ocupados; Internet entra por Cloudflare Tunnel.
docker run -d --name pikka-web    --restart unless-stopped --network "$NETWORK" pikka/web:1.0 >/dev/null
docker run -d --name pikka-mobile --restart unless-stopped --network "$NETWORK" pikka/mobile-web:1.0 >/dev/null
# pikka-tunnel se crea una sola vez con su token; aquí se verifica y se conecta a la red de Pikka.
docker inspect -f "{{.State.Running}}" pikka-tunnel | grep -q true || { echo "pikka-tunnel no está en ejecución"; exit 1; }
docker network connect "$NETWORK" pikka-tunnel 2>/dev/null || echo "pikka-tunnel ya está en la red $NETWORK"
echo "DESPLIEGUE EN EL SERVIDOR COMPLETADO"
'''
                sh '''
                H="$CLOUD_HOST"; R='~/pikka_gestion'
                # Reintenta un comando SSH corto si la conexión falla.
                rssh() { for i in 1 2 3 4 5; do ssh -o ConnectTimeout=15 "$H" "$@" && return 0; echo "(SSH falló, reintento $i)"; sleep 5; done; return 1; }

                rssh "cd $R/Backend-Pikka && git fetch -q origin && git checkout -q --detach $GIT_COMMIT && git log --oneline -1"
                rssh "cd $R/Frontend-Pikka && git fetch -q origin && git checkout -q --detach origin/$FRONTEND_BRANCH && git log --oneline -1"
                scp -q .env "$H:pikka_gestion/Backend-Pikka/.env" && rssh "chmod 600 $R/Backend-Pikka/.env"
                scp -q cd/deploy-nube.sh cd/mobile-web.Dockerfile cd/mobile-web.Dockerfile.dockerignore "$H:pikka_gestion/"
                rssh "cd $R && rm -f pikka-cd.status pikka-cd.log && SERVICES='$SERVICES' NETWORK='$NETWORK' MOBILE_API='$MOBILE_API' setsid nohup sh deploy-nube.sh > pikka-cd.log 2>&1 < /dev/null & echo 'Despliegue lanzado en el servidor'"

                visto=0
                for i in $(seq 1 240); do
                    sleep 10
                    total=$(ssh -o ConnectTimeout=15 "$H" "wc -l < $R/pikka-cd.log" 2>/dev/null) || { echo "(sin conexión SSH; el despliegue sigue en el servidor)"; continue; }
                    if [ "$total" -gt "$visto" ]; then
                        ssh -o ConnectTimeout=15 "$H" "sed -n '$((visto+1)),${total}p' $R/pikka-cd.log" 2>/dev/null && visto=$total
                    fi
                    estado=$(ssh -o ConnectTimeout=15 "$H" "cat $R/pikka-cd.status 2>/dev/null" 2>/dev/null || true)
                    if [ -n "$estado" ]; then
                        ssh -o ConnectTimeout=15 "$H" "sed -n '$((visto+1)),\\$p' $R/pikka-cd.log" 2>/dev/null || true
                        [ "$estado" = 0 ] && { echo "Despliegue en la nube: OK"; exit 0; }
                        echo "Despliegue en la nube: FALLÓ (código $estado)"; exit 1
                    fi
                done
                echo "Tiempo de espera agotado (40 min)"; exit 1
                '''
            }
        }

        stage('Prueba de humo') {
            steps {
                withEnv(["RUN=${params.TARGET == 'nube' ? 'ssh -o ConnectTimeout=15 ' + params.CLOUD_HOST : ''}"]) {
                    sh '''
                    for c in pikka-web pikka-mobile; do
                        ok=0
                        for i in $(seq 1 30); do
                            if $RUN docker exec $c wget -qO- http://127.0.0.1/ready 2>/dev/null | grep -q ready; then ok=1; break; fi
                            sleep 5
                        done
                        [ "$ok" = 1 ] || { echo "El gateway no respondio /ready a traves de $c"; exit 1; }
                        echo "Gateway OK a traves de Nginx ($c)"
                    done
                    '''
                }
                script {
                    if (params.TARGET == 'nube') {
                        sh '''
                        for u in "$PUBLIC_WEB/ready" "$PUBLIC_MOBILE/ready" "$PUBLIC_API/health"; do
                            ok=0
                            for i in $(seq 1 24); do
                                code=$(curl -s -o /dev/null -w "%{http_code}" -A pikka-jenkins "$u" || true)
                                [ "$code" = 200 ] && { ok=1; break; }
                                sleep 5
                            done
                            [ "$ok" = 1 ] || { echo "FALLA $u (HTTP $code)"; exit 1; }
                            echo "OK $u -> 200 (HTTPS por Cloudflare)"
                        done
                        '''
                    }
                }
            }
        }
    }

    post {
        failure { sh 'docker ps -a --format "table {{.Names}}\\t{{.Status}}" || true' }
        always  { sh 'rm -f .env Dockerfile.test ~/.ssh/pikka_oracle ~/.ssh/config || true' }
    }
}
