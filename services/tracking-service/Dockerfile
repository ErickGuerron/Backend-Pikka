# Construir desde la raíz del repositorio:
#   docker build -f services/tracking-service/Dockerfile .
# Necesita contracts/ (contrato tracking.v1) y libs/osrm/ (estimador de rutas): el módulo los declara
# con replace locales.
FROM golang:1.26-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOWORK=off

COPY contracts/go.mod contracts/go.sum ./contracts/
COPY libs/osrm/go.mod ./libs/osrm/
COPY services/tracking-service/go.mod services/tracking-service/go.sum ./services/tracking-service/
RUN cd services/tracking-service && go mod download

COPY contracts/ ./contracts/
COPY libs/osrm/ ./libs/osrm/
COPY services/tracking-service/ ./services/tracking-service/
RUN cd services/tracking-service && go build -trimpath -ldflags="-s -w" -o /out/service ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/service /service
USER nonroot:nonroot
ENTRYPOINT ["/service"]
