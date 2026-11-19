package position

import (
	"context"

	"github.com/ErickGuerron/Backend-Pikka/services/tracking-service/internal/domain"
)

// FreshPosition returns the driver's latest fix when it is still inside the fresh window. A missing fix
// is ErrNoPosition and an old one is ErrStalePosition: the caller decides what to show, never a guess.
func (t *Tracker) FreshPosition(ctx context.Context, driverID string) (domain.Position, error) {
	last, ok, err := t.store.Latest(ctx, driverID)
	if err != nil {
		return domain.Position{}, err
	}
	if !ok {
		return domain.Position{}, ErrNoPosition
	}
	if !last.IsFresh(t.now()) {
		return domain.Position{}, ErrStalePosition
	}
	return last, nil
}
