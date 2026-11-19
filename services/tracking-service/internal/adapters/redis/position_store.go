// Package redis stores the latest driver position in Redis, one expiring key per driver.
package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/domain"
)

const keyPrefix = "tracking:driver:"

// PositionStore implements the application's PositionStore on Redis.
type PositionStore struct {
	client *goredis.Client
}

// NewPositionStore wraps a Redis client. The client's lifecycle belongs to the caller.
func NewPositionStore(client *goredis.Client) *PositionStore {
	return &PositionStore{client: client}
}

// stored is the on-disk shape. It is internal so the Redis format can change without touching the domain.
type stored struct {
	Latitude   float64   `json:"lat"`
	Longitude  float64   `json:"lon"`
	RecordedAt time.Time `json:"recorded_at"`
}

func key(driverID string) string {
	return keyPrefix + driverID
}

// Save overwrites the driver's latest fix and sets it to expire after ttl.
func (s *PositionStore) Save(ctx context.Context, p domain.Position, ttl time.Duration) error {
	payload, err := json.Marshal(stored{Latitude: p.Latitude, Longitude: p.Longitude, RecordedAt: p.RecordedAt.UTC()})
	if err != nil {
		return fmt.Errorf("encode position: %w", err)
	}
	if err := s.client.Set(ctx, key(p.DriverID), payload, ttl).Err(); err != nil {
		return fmt.Errorf("redis set position: %w", err)
	}
	return nil
}

// Latest returns the driver's stored fix. ok is false when there is none or it has expired.
func (s *PositionStore) Latest(ctx context.Context, driverID string) (domain.Position, bool, error) {
	raw, err := s.client.Get(ctx, key(driverID)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return domain.Position{}, false, nil
	}
	if err != nil {
		return domain.Position{}, false, fmt.Errorf("redis get position: %w", err)
	}
	var v stored
	if err := json.Unmarshal(raw, &v); err != nil {
		return domain.Position{}, false, fmt.Errorf("decode position: %w", err)
	}
	return domain.Position{DriverID: driverID, Latitude: v.Latitude, Longitude: v.Longitude, RecordedAt: v.RecordedAt}, true, nil
}
