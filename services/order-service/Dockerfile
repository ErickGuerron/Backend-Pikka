# Construir desde la raíz del repositorio:
#   docker build -f services/order-service/Dockerfile .
FROM golang:1.26-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOWORK=off

COPY contracts/go.mod contracts/go.sum ./contracts/
COPY services/order-service/go.mod services/order-service/go.sum ./services/order-service/
RUN cd services/order-service && go mod download

COPY contracts/ ./contracts/
COPY services/order-service/ ./services/order-service/
RUN cd services/order-service && go build -trimpath -ldflags="-s -w" -o /out/service ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/service /service
USER nonroot:nonroot
ENTRYPOINT ["/service"]