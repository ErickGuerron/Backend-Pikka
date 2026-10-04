module github.com/ErickGuerron/Backend-Pikka/services/api-gateway

go 1.26.0

require (
	github.com/ErickGuerron/Backend-Pikka/contracts v0.0.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/labstack/echo/v4 v4.16.0
	github.com/stretchr/testify v1.12.1
	golang.org/x/time v0.16.0
	google.golang.org/grpc v1.85.0-dev.0.20260825072537-93e31b48545e
)

require (
	github.com/labstack/gommon v0.5.0 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260928230214-8a89bd6388cc // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

replace github.com/ErickGuerron/Backend-Pikka/contracts => ../../contracts
