module github.com/ErickGuerron/Backend-Pikka/services/routing-service

go 1.26.0

replace github.com/ErickGuerron/Backend-Pikka/contracts => ../../contracts

require (
	github.com/ErickGuerron/Backend-Pikka/contracts v0.0.0-00010101000000-000000000000
	google.golang.org/grpc v1.85.0-dev.0.20260825072537-93e31b48545e
)

require (
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260928230214-8a89bd6388cc // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)
