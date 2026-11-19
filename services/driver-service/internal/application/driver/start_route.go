package driver

import (
	"context"
	"strings"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

// StartRoute marca al repartidor como ON_ROUTE en routeID. Se llama con el primer PICKED_UP de la ruta.
// Es idempotente: si ya está ON_ROUTE en esa misma ruta, no hace nada y responde éxito.
func (s *Service) StartRoute(ctx context.Context, id, routeID string) (*domain.Driver, error) {
	return s.mutate(ctx, id, func(d *domain.Driver, now time.Time) error {
		if d.Status == domain.StatusOnRoute && d.RouteID == strings.TrimSpace(routeID) {
			return nil
		}
		return d.StartRoute(routeID, now)
	})
}
