package http_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDriverRouteRead_ReportsTheRouteCapacity: la app del repartidor necesita el maximo de pedidos de su ruta
// para mostrar cuantos le caben. Viene de Order Service (zona de la ruta).
func TestDriverRouteRead_ReportsTheRouteCapacity(t *testing.T) {
	res := twoStops()
	res.MaxOrders = 10
	orders := &readStopsOrders{res: res}
	drivers := &readRouteDrivers{routeID: readRouteID}

	rec := readRouteGet(t, orders, drivers, "DRIVER")

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		MaxOrders int `json:"max_orders"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, 10, body.MaxOrders)
}
