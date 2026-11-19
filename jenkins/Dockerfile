# Jenkins LTS con Docker CLI, Compose y cliente SSH. Usa el Docker del equipo (se le pasa el socket
# al ejecutarlo) y, para el despliegue en la nube, un Docker remoto por SSH (DOCKER_HOST=ssh://...).
FROM jenkins/jenkins:lts-jdk21
USER root
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl gnupg git openssh-client \
 && install -m 0755 -d /etc/apt/keyrings \
 && curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc \
 && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian $(. /etc/os-release && echo $VERSION_CODENAME) stable" > /etc/apt/sources.list.d/docker.list \
 && apt-get update \
 && apt-get install -y --no-install-recommends docker-ce-cli docker-compose-plugin docker-buildx-plugin \
 && rm -rf /var/lib/apt/lists/*
# Plugins necesarios, instalados al construir la imagen para que cualquier integrante obtenga el mismo Jenkins.
COPY plugins.txt /usr/share/jenkins/ref/plugins.txt
USER jenkins
RUN jenkins-plugin-cli --plugin-file /usr/share/jenkins/ref/plugins.txt
USER root
# Se queda como root para poder usar /var/run/docker.sock en Docker Desktop (entorno de práctica).
