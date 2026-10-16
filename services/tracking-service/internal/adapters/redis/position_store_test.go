package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/domain"
)

var t0 = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

func newStore(t *testing.T) (*PositionStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewPositionStore(client), mr
}

func TestSaveThenLatestRoundTrips(t *testing.T) {
	store, _ := newStore(t)
	ctx := context.Background()
	in, _ := domain.NewPosition("driver-1", -0.18, -78.48, t0, t0)

	if err := store.Save(ctx, in, domain.FreshWindow); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, ok, err := store.Latest(ctx, "driver-1")
	if err != nil || !ok {
		t.Fatalf("Latest ok=%v err=%v", ok, err)
	}
	if got.DriverID != in.DriverID || got.Latitude != in.Latitude || got.Longitude != in.Longitude || !got.RecordedAt.Equal(in.RecordedAt) {
		t.Fatalf("got %+v, want %+v", got, in)
	}
}

func TestLatestMissingDriverReportsNotFound(t *testing.T) {
	store, _ := newStore(t)

	_, ok, err := store.Latest(context.Background(), "nobody")
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want not found without error", ok, err)
	}
}

// The fix expires from the store after the fresh window, so an old position is not kept forever.
func TestSavedPositionExpiresAfterTTL(t *testing.T) {
	store, mr := newStore(t)
	ctx := context.Background()
	in, _ := domain.NewPosition("driver-1", 0, 0, t0, t0)
	if err := store.Save(ctx, in, domain.FreshWindow); err != nil {
		t.Fatal(err)
	}

	mr.FastForward(domain.FreshWindow + time.Second)

	if _, ok, _ := store.Latest(ctx, "driver-1"); ok {
		t.Fatal("position must expire after the fresh window")
	}
}

// Keys are namespaced so tracking data cannot collide with other services sharing the Redis instance.
func TestKeyIsNamespacedByDriver(t *testing.T) {
	store, mr := newStore(t)
	in, _ := domain.NewPosition("driver-1", 0, 0, t0, t0)
	if err := store.Save(context.Background(), in, domain.FreshWindow); err != nil {
		t.Fatal(err)
	}
	if !mr.Exists("tracking:driver:driver-1") {
		t.Fatalf("expected key tracking:driver:driver-1, have %v", mr.Keys())
	}
}
