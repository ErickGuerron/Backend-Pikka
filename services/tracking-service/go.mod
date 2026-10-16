module github.com/ErickGuerron/Backend-Pikka/services/tracking-service

go 1.26.0

require (
	github.com/ErickGuerron/Backend-Pikka/libs/osrm v0.0.0-00010101000000-000000000000
	github.com/alicebob/miniredis/v2 v2.40.0
	github.com/redis/go-redis/v9 v9.23.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.uber.org/atomic v1.12.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace github.com/ErickGuerron/Backend-Pikka/libs/osrm => ../../libs/osrm
