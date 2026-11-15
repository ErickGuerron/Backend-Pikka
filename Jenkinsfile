pipeline {
    agent any
    options { timestamps(); disableConcurrentBuilds() }

    parameters {
        choice(name: 'TARGET', choices: ['local', 'nube'], description: 'Entorno de despliegue')
        string(name: 'FRONTEND_BRANCH', defaultValue: 'develop', description: 'Rama de Frontend-Pikka que se construye')
        string(name: 'CLOUD_HOST', defaultValue: '', description: 'Solo nube: usuario@IP de la VM, por ejemplo ubuntu@129.80.10.20')
    }

    environment {
        COMPOSE = 'docker compose -f docker-compose.yml -f docker-compose.ci.yml'
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
                    env.WEB_PORT = params.TARGET == 'nube' ? '80' : '5173'
                }
                withCredentials([file(credentialsId: env.ENV_CRED, variable: 'ENV_FILE')]) {
                    sh 'cp "$ENV_FILE" .env'
                }
            }
        }

        stage('Pruebas unitarias') {
            steps {
                // Las pruebas corren dentro de un docker build: el contexto lo envia el cliente, sin montar carpetas del equipo
                writeFile file: 'Dockerfile.test', text: '''FROM golang:1.26-alpine
ENV CGO_ENABLED=0
WORKDIR /src
COPY . .
ARG SERVICE
RUN cd services/${SERVICE} && go test ./...
'''
                sh '''
                for s in auth-service api-gateway order-service zone-service driver-service routing-service tracking-service; do
                    docker build -f Dockerfile.test --build-arg SERVICE=$s -t pikka-test:$s . || exit 1
                done
                '''
            }
        }

        stage('Acceso a la nube') {
            when { expression { params.TARGET == 'nube' } }
            steps {
                withCredentials([sshUserPrivateKey(credentialsId: 'oracle-ssh', keyFileVariable: 'KEY')]) {
                    sh '''
                    mkdir -p ~/.ssh
                    cp "$KEY" ~/.ssh/pikka_oracle && chmod 600 ~/.ssh/pikka_oracle
                    printf "Host *\\n  IdentityFile ~/.ssh/pikka_oracle\\n  StrictHostKeyChecking accept-new\\n" > ~/.ssh/config
                    '''
                }
            }
        }

        stage('Desplegar backend') {
            steps {
                // La VM de Oracle es x86_64, igual que el equipo local: usa la imagen oficial de PostGIS
                withEnv(env.DH ? ["DOCKER_HOST=${env.DH}"] : []) {
                    sh '$COMPOSE up -d --build'
                }
            }
        }

        stage('Frontend web (Nginx)') {
            steps {
                dir('frontend') {
                    git url: 'https://github.com/ErickGuerron/Frontend-Pikka.git', branch: params.FRONTEND_BRANCH
                }
                withEnv(env.DH ? ["DOCKER_HOST=${env.DH}"] : []) {
                    dir('frontend/apps') {
                        sh '''
                        docker build -f web/Dockerfile -t pikka/web:1.0 .
                        docker rm -f pikka-web || true
                        docker run -d --name pikka-web --restart unless-stopped --network last-mile-logistics_default -p ${WEB_PORT}:80 pikka/web:1.0
                        '''
                    }
                }
            }
        }

        stage('Prueba de humo') {
            steps {
                withEnv(env.DH ? ["DOCKER_HOST=${env.DH}"] : []) {
                    sh '''
                    for i in $(seq 1 30); do
                        if docker exec pikka-web wget -qO- http://127.0.0.1/ready; then echo; echo "Gateway OK a traves de Nginx"; exit 0; fi
                        sleep 5
                    done
                    echo "El gateway no respondio /ready"; exit 1
                    '''
                }
            }
        }
    }

    post {
        failure { sh 'docker ps -a || true' }
        always  { sh 'rm -f .env Dockerfile.test ~/.ssh/pikka_oracle ~/.ssh/config || true' }
    }
}
