GO_MODULES := services/auth-service services/api-gateway
LINT := golangci-lint

.PHONY: help up down logs test test-integration lint fmt proto openapi-lint

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

lint: ## go vet + golangci-lint
	@for m in $(GO_MODULES); do echo "== $$m"; (cd $$m && go vet ./... && $(LINT) run --config ../../.golangci.yml ./...) || exit 1; done

fmt: ## Formatea el código Go
	@for m in $(GO_MODULES); do (cd $$m && gofmt -w .); done

proto: ## Regenera el código Go de los contratos gRPC (requiere buf, protoc-gen-go y protoc-gen-go-grpc)
	cd contracts && buf lint && buf generate

openapi-lint: ## Valida el contrato OpenAPI
	npx --yes @redocly/cli@1.34.0 lint --config contracts/openapi/redocly.yaml contracts/openapi/openapi.yaml
