//go:build integration

package redis

import (
	"context"
	"os"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/domain"
)

// Requires REDIS_URL pointing at the Redis of docker compose, e.g. redis://:<password>@127.0.0.1:6379/0.
// Run with: go test -tags integration ./internal/adapters/redis/
func TestRedisRoundTripAgainstRealServer(t *testing.T) {
	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL not set")
	}
	opts, err := goredis.ParseURL(url)
	if err != nil {
		t.Fatalf("ParseURL: %v", err)
	}
	client := goredis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })

	store := NewPositionStore(client)
	ctx := context.Background()
	driverID := "integration-driver"
	recorded := time.Now().UTC().Truncate(time.Second)
	in, err := domain.NewPosition(driverID, -0.18, -78.48, recorded, recorded)
	if err != nil {
		t.Fatalf("NewPosition: %v", err)
	}
	t.Cleanup(func() { _ = client.Del(ctx, key(driverID)).Err() })

	if err := store.Save(ctx, in, domain.FreshWindow); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, ok, err := store.Latest(ctx, driverID)
	if err != nil || !ok {
		t.Fatalf("Latest ok=%v err=%v", ok, err)
	}
	if got.Latitude != in.Latitude || got.Longitude != in.Longitude || !got.RecordedAt.Equal(recorded) {
		t.Fatalf("got %+v, want %+v", got, in)
	}
	ttl, err := client.TTL(ctx, key(driverID)).Result()
	if err != nil || ttl <= 0 || ttl > domain.FreshWindow {
		t.Fatalf("ttl = %v err=%v, want 0 < ttl <= %v", ttl, err, domain.FreshWindow)
	}
}
