#!/bin/bash
# Crea un esquema y un usuario por microservicio (sección 18 de la base técnica).
# Cada usuario es dueño únicamente de su esquema y no puede leer los demás.
# Se ejecuta una sola vez, cuando el volumen de PostgreSQL está vacío.
set -euo pipefail

create_service() {
  local schema="$1" user="$2" password="$3"
  if [ -z "$password" ]; then
    echo "missing password for $user" >&2
    exit 1
  fi
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
    -v schema="$schema" -v svc_user="$user" -v svc_password="$password" <<-'SQL'
	CREATE ROLE :"svc_user" LOGIN PASSWORD :'svc_password';
	CREATE SCHEMA :"schema" AUTHORIZATION :"svc_user";
	REVOKE ALL ON SCHEMA :"schema" FROM PUBLIC;
	GRANT CONNECT ON DATABASE :"DBNAME" TO :"svc_user";
	ALTER ROLE :"svc_user" SET search_path = :"schema", public;
SQL
}

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-SQL
	CREATE EXTENSION IF NOT EXISTS postgis;
	REVOKE CREATE ON SCHEMA public FROM PUBLIC;
	REVOKE ALL ON DATABASE "$POSTGRES_DB" FROM PUBLIC;
SQL

create_service auth     auth_service_user     "${AUTH_DB_PASSWORD:-}"
create_service orders   order_service_user    "${ORDER_DB_PASSWORD:-}"
create_service zones    zone_service_user     "${ZONE_DB_PASSWORD:-}"
create_service routing  routing_service_user  "${ROUTING_DB_PASSWORD:-}"
create_service drivers  driver_service_user   "${DRIVER_DB_PASSWORD:-}"
create_service tracking tracking_service_user "${TRACKING_DB_PASSWORD:-}"
