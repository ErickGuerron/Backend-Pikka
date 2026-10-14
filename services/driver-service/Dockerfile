# Construir desde la raíz del repositorio:
#   docker build -f services/driver-service/Dockerfile .
# El servicio no depende de contracts/: su módulo se compila solo, con GOWORK=off.
FROM golang:1.26-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOWORK=off

COPY services/driver-service/go.mod services/driver-service/go.sum ./services/driver-service/
RUN cd services/driver-service && go mod download

COPY services/driver-service/ ./services/driver-service/
RUN cd services/driver-service && go build -trimpath -ldflags="-s -w" -o /out/service ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/service /service
USER nonroot:nonroot
ENTRYPOINT ["/service"]
