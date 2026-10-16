GO_MODULES := services/auth-service services/api-gateway services/order-service services/zone-service services/driver-service services/routing-service services/tracking-service
LINT := golangci-lint

# Carga las variables de .env (igual que docker compose) para que las recetas
# usen las mismas contraseñas que el contenedor. Sin .env no falla.
-include .env
export

# En Windows, make usa cmd.exe por defecto, que no entiende los bucles ni la
# sintaxis POSIX de estas recetas. Usamos el sh de Git for Windows. La ruta es
# la forma corta 8.3 porque GNU make no admite espacios en SHELL.
ifeq ($(OS),Windows_NT)
SHELL := C:/PROGRA~1/Git/bin/sh.exe
endif

.PHONY: help up down logs test test-integration lint fmt proto openapi-lint perf-smoke security-scan osrm-prepare osrm-up osrm-down osrm-test

help: ## Muestra esta ayuda
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

up: ## Levanta PostgreSQL+PostGIS, Redis y los servicios
	@test -f .env || cp .env.example .env
	docker compose up -d --build

down: ## Detiene el entorno (agrega -v a mano para borrar datos)
	docker compose down

logs: ## Sigue los logs de los servicios Go
	docker compose logs -f api-gateway auth-service

test: ## Pruebas unitarias de todos los módulos
	@for m in $(GO_MODULES); do echo "== $$m"; (cd $$m && go test -race ./...) || exit 1; done

test-integration: ## Pruebas de integración contra el PostgreSQL de docker compose
	cd services/auth-service && AUTH_TEST_DATABASE_URL="postgres://auth_service_user:$${AUTH_DB_PASSWORD:-change_me_auth}@localhost:5432/last_mile?sslmode=disable&search_path=auth" go test -tags integration -count=1 ./...
	cd services/order-service && ORDER_TEST_DATABASE_URL="postgres://order_service_user:$${ORDER_DB_PASSWORD:-change_me_orders}@localhost:5432/last_mile?sslmode=disable&search_path=orders,public" go test -tags integration -count=1 ./...
	cd services/driver-service && DRIVER_TEST_DATABASE_URL="postgres://driver_service_user:$${DRIVER_DB_PASSWORD:-change_me_drivers}@localhost:5432/last_mile?sslmode=disable&search_path=drivers,public" go test -tags integration -count=1 ./...

lint: ## go vet + golangci-lint
	@for m in $(GO_MODULES); do echo "== $$m"; (cd $$m && go vet ./... && $(LINT) run --config ../../.golangci.yml ./...) || exit 1; done

fmt: ## Formatea el código Go
	@for m in $(GO_MODULES); do (cd $$m && gofmt -w .); done

proto: ## Regenera el código Go de los contratos gRPC (requiere buf, protoc-gen-go y protoc-gen-go-grpc)
	cd contracts && buf lint && buf generate

openapi-lint: ## Valida el contrato OpenAPI
	npx --yes @redocly/cli@1.34.0 lint --config contracts/openapi/redocly.yaml contracts/openapi/openapi.yaml

perf-smoke: ## Smoke de rendimiento con k6 (requiere `make up`)
	docker compose --profile perf run --rm k6

security-scan: ## Escaneo activo OWASP ZAP sobre el contrato OpenAPI (requiere `make up`)
	docker compose --profile security run --rm zap

osrm-prepare: ## Descarga el mapa de Ecuador y genera los datos de OSRM (una vez; -Force regenera)
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/osrm-prepare-ecuador.ps1

osrm-up: ## Levanta el servidor OSRM de Ecuador en 127.0.0.1:5000 (requiere `make osrm-prepare`)
	docker compose --profile osrm up -d osrm

osrm-down: ## Detiene el servidor OSRM
	docker compose --profile osrm stop osrm

osrm-test: ## Pruebas de integración del módulo libs/osrm contra el servidor levantado con `make osrm-up`
	cd libs/osrm && OSRM_URL="http://127.0.0.1:5000" go test -race -tags integration -count=1 ./...
