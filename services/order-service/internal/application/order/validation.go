package order

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// placement resuelve dónde está el pedido y qué reglas no cumple (ADR 0007, section 3).
// Las mismas reglas las usan el alta del cliente y la corrección del operador.
type placement struct {
	zones    ports.ZoneClassifier
	geocoder ports.Geocoder
}

// coordinates es un punto conocido. Si es nil, la dirección se geocodifica en el backend.
type coordinates struct {
	latitude  float64
	longitude float64
}

// locate devuelve el punto del pedido. located es false si la dirección no se pudo
// localizar: eso es una revisión, no un error. Un fallo del geocodificador sí es error.
func (p placement) locate(ctx context.Context, address string, known *coordinates) (coordinates, bool, error) {
	if known != nil {
		return *known, true, nil
	}
	lat, lng, found, err := p.geocoder.Locate(ctx, address)
	if err != nil {
		return coordinates{}, false, err
	}
	if !found {
		return coordinates{}, false, nil
	}
	return coordinates{latitude: lat, longitude: lng}, true, nil
}

// reasons aplica las reglas automáticas. Devuelve el id de la zona si la hay.
// Un fallo de Zone Service es error: el pedido no se decide a ciegas.
func (p placement) reasons(ctx context.Context, point coordinates, located, blocked bool) ([]domain.ReviewReason, string, error) {
	var reasons []domain.ReviewReason
	if blocked {
		reasons = append(reasons, domain.ReviewCustomerBlocked)
	}
	if !located {
		return append(reasons, domain.ReviewAddressNotLocated), "", nil
	}
	zoneID, err := p.zones.ClassifyPoint(ctx, point.latitude, point.longitude)
	switch {
	case errors.Is(err, domain.ErrZoneNotCovered):
		return append(reasons, domain.ReviewZoneNotCovered), "", nil
	case err != nil:
		return nil, "", fmt.Errorf("classify delivery point: %w", err)
	}
	return reasons, zoneID, nil
}

// reviewDetail resume los motivos para el aviso al cliente.
func reviewDetail(reasons []domain.ReviewReason) string {
	parts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		parts = append(parts, string(reason))
	}
	return strings.Join(parts, ", ")
}
