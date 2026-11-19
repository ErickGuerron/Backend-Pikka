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

        stage('Desplegar backend') {
            steps {
                // La VM de Oracle es x86_64, igual que el equipo local: usa la imagen oficial de PostGIS
                withEnv(env.DH ? ["DOCKER_HOST=${env.DH}"] : []) {
                    script {
                        if (params.TARGET == 'nube') {
                            // Una imagen a la vez: el servidor (2 vCPU) atiende otro sistema en producción.
                            // La compilación remota viaja por SSH: si la conexión se corta, se reintenta esa imagen.
                            sh '''
                            for s in $SERVICES postgres redis; do
                                ok=0
                                for intento in 1 2 3; do
                                    echo "== build $s (intento $intento)"
                                    if $COMPOSE build $s; then ok=1; break; fi
                                    echo "Se cortó la compilación remota de $s; reintentando en 10 s..."; sleep 10
                                done
                                [ "$ok" = 1 ] || { echo "No se pudo construir $s en la nube"; exit 1; }
                            done
                            '''
                            sh '$COMPOSE up -d'
                        } else {
                            sh '$COMPOSE up -d --build'
                        }
                    }
                }
            }
        }

        stage('Frontend web y app móvil (Nginx)') {
            steps {
                dir('frontend') {
                    git url: 'https://github.com/ErickGuerron/Frontend-Pikka.git', branch: params.FRONTEND_BRANCH
                }
                // Imagen de la app móvil (Expo exportada a web + Nginx). El contexto es apps/ porque importa apps/shared.
                writeFile file: 'frontend/apps/mobile-web.Dockerfile', text: '''FROM node:22-alpine AS build
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
                writeFile file: 'frontend/apps/mobile-web.Dockerfile.dockerignore', text: '''**/node_modules
**/dist
**/coverage
**/.env
**/.env.local
**/.expo
'''
                withEnv(env.DH ? ["DOCKER_HOST=${env.DH}"] : []) {
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
        }

        stage('Túnel de Cloudflare') {
            when { expression { params.TARGET == 'nube' } }
            steps {
                withEnv(["DOCKER_HOST=${env.DH}"]) {
                    // pikka-tunnel se crea una sola vez en el servidor con su token; aquí se verifica y se conecta a la red.
                    sh '''
                    docker inspect -f "{{.State.Running}}" pikka-tunnel | grep -q true || { echo "pikka-tunnel no está en ejecución"; exit 1; }
                    docker network connect $NETWORK pikka-tunnel 2>/dev/null || echo "pikka-tunnel ya está en la red $NETWORK"
                    '''
                }
            }
        }

        stage('Prueba de humo') {
            steps {
                withEnv(env.DH ? ["DOCKER_HOST=${env.DH}"] : []) {
                    sh '''
                    for c in pikka-web pikka-mobile; do
                        ok=0
                        for i in $(seq 1 30); do
                            if docker exec $c wget -qO- http://127.0.0.1/ready 2>/dev/null | grep -q ready; then ok=1; break; fi
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
        failure { sh 'docker ps -a || true' }
        always  { sh 'rm -f .env Dockerfile.test ~/.ssh/pikka_oracle ~/.ssh/config || true' }
    }
}
