//go:build integration

package postgres_test

import (
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

// onlyIDs conserva los repartidores de la lista cuyo id pertenece a la prueba. El esquema de cada prueba
// trae el repartidor semilla de la migracion 000003, asi que las pruebas filtran por sus propios ids.
func onlyIDs(list []*domain.Driver, ids ...string) []*domain.Driver {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	var out []*domain.Driver
	for _, d := range list {
		if want[d.ID] {
			out = append(out, d)
		}
	}
	return out
}
