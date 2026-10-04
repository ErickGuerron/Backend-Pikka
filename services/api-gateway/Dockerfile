# Construir desde la raíz del repositorio:
#   docker build -f services/api-gateway/Dockerfile .
FROM golang:1.26-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOWORK=off

COPY contracts/go.mod contracts/go.sum ./contracts/
COPY services/api-gateway/go.mod services/api-gateway/go.sum ./services/api-gateway/
RUN cd services/api-gateway && go mod download

COPY contracts/ ./contracts/
COPY services/api-gateway/ ./services/api-gateway/
RUN cd services/api-gateway && go build -trimpath -ldflags="-s -w" -o /out/service ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/service /service
USER nonroot:nonroot
ENTRYPOINT ["/service"]
