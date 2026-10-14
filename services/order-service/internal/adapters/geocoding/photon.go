// Package geocoding convierte direcciones en coordenadas con Photon (OpenStreetMap).
// La geocodificación vive en el backend: el cliente no decide las coordenadas que
// se validan contra las zonas (BASE_TECNICA §17).
package geocoding

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// maxResponseBytes limita la respuesta del proveedor.
const maxResponseBytes = 1 << 20

// Photon consulta una instancia de Photon. En producción debe ser una instancia
// propia (imagen Docker oficial, datos de Ecuador): la pública tiene límites de uso.
type Photon struct {
	baseURL string
	bbox    string
	client  *http.Client
}

var _ ports.Geocoder = (*Photon)(nil)

// NewPhoton configura el cliente. bbox acota la búsqueda a un área (minLon,minLat,maxLon,maxLat).
func NewPhoton(baseURL, bbox string, timeout time.Duration) *Photon {
	return &Photon{baseURL: strings.TrimRight(baseURL, "/"), bbox: bbox, client: &http.Client{Timeout: timeout}}
}

type photonResponse struct {
	Features []struct {
		Geometry struct {
			// GeoJSON: [longitud, latitud].
			Coordinates []float64 `json:"coordinates"`
		} `json:"geometry"`
	} `json:"features"`
}

// Locate devuelve la primera coincidencia. Sin coincidencias, found es false y no es error.
func (p *Photon) Locate(ctx context.Context, address string) (float64, float64, bool, error) {
	query := url.Values{}
	query.Set("q", address)
	query.Set("limit", "1")
	query.Set("lang", "es")
	if p.bbox != "" {
		query.Set("bbox", p.bbox)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/api/?"+query.Encode(), nil)
	if err != nil {
		return 0, 0, false, fmt.Errorf("photon request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	res, err := p.client.Do(req)
	if err != nil {
		return 0, 0, false, domain.ErrGeocoderUnavailable
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return 0, 0, false, domain.ErrGeocoderUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes))
	if err != nil {
		return 0, 0, false, domain.ErrGeocoderUnavailable
	}

	var parsed photonResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, 0, false, domain.ErrGeocoderUnavailable
	}
	for _, feature := range parsed.Features {
		if len(feature.Geometry.Coordinates) == 2 {
			return feature.Geometry.Coordinates[1], feature.Geometry.Coordinates[0], true, nil
		}
	}
	return 0, 0, false, nil
}
