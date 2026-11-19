# Construir desde la raíz del repositorio:
#   docker build -f services/driver-service/Dockerfile .
# Necesita contracts/: el módulo declara el contrato drivers.v1 con un replace local.
FROM golang:1.26-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOWORK=off

COPY contracts/go.mod contracts/go.sum ./contracts/
COPY services/driver-service/go.mod services/driver-service/go.sum ./services/driver-service/
RUN cd services/driver-service && go mod download

COPY contracts/ ./contracts/
COPY services/driver-service/ ./services/driver-service/
RUN cd services/driver-service && go build -trimpath -ldflags="-s -w" -o /out/service ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/service /service
USER nonroot:nonroot
ENTRYPOINT ["/service"]
