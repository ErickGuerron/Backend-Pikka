module github.com/ErickGuerron/Backend-Pikka/services/auth-service

go 1.26.0

require (
	github.com/ErickGuerron/Backend-Pikka/contracts v0.0.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/golang-migrate/migrate/v4 v4.20.1
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.11.0
	github.com/labstack/echo/v4 v4.16.0
	github.com/stretchr/testify v1.12.1
	golang.org/x/crypto v0.57.0
	google.golang.org/grpc v1.84.0
)

require (
	github.com/jackc/pgerrcode v0.0.0-20250907135507-afb5586c32a6 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/labstack/gommon v0.5.0 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260928230214-8a89bd6388cc // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

replace github.com/ErickGuerron/Backend-Pikka/contracts => ../../contracts
