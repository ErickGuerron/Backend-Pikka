# Construir desde la raíz del repositorio:
#   docker build -f services/routing-service/Dockerfile .
# Necesita contracts/ y libs/osrm/: el módulo los referencia con replace locales.
FROM golang:1.26-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOWORK=off

COPY contracts/go.mod contracts/go.sum ./contracts/
COPY libs/osrm/go.mod ./libs/osrm/
COPY services/routing-service/go.mod services/routing-service/go.sum ./services/routing-service/
RUN cd services/routing-service && go mod download

COPY contracts/ ./contracts/
COPY libs/osrm/ ./libs/osrm/
COPY services/routing-service/ ./services/routing-service/
RUN cd services/routing-service && go build -trimpath -ldflags="-s -w" -o /out/service ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/service /service
USER nonroot:nonroot
ENTRYPOINT ["/service"]
