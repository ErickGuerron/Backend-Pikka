package http_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// La app del repartidor envía la posición anidada en "location", como declara el OpenAPI. Antes el
// Gateway solo leía latitude/longitude en la raíz y rechazaba toda entrega con 400.
func TestDriverResult_AcceptsLocationObjectFromTheContract(t *testing.T) {
	orders, drivers := resultFakes()
	body := `{"outcome":"DELIVERED","location":{"latitude":-1.2678,"longitude":-78.6242}}`

	rec := do(stopRouter(orders, drivers), http.MethodPost, resultPath, body, driverStopHeaders(t))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NotNil(t, drivers.registerReq)
	assert.InDelta(t, -1.2678, drivers.registerReq.GetLatitude(), 1e-9)
	assert.InDelta(t, -78.6242, drivers.registerReq.GetLongitude(), 1e-9)
}

func TestDriverResult_LocationObjectWinsOverFlatFields(t *testing.T) {
	orders, drivers := resultFakes()
	body := `{"outcome":"DELIVERED","location":{"latitude":-1.5,"longitude":-78.5},"latitude":9,"longitude":9}`

	rec := do(stopRouter(orders, drivers), http.MethodPost, resultPath, body, driverStopHeaders(t))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.InDelta(t, -1.5, drivers.registerReq.GetLatitude(), 1e-9)
}

func TestDriverResult_RejectsIncompleteLocation(t *testing.T) {
	orders, drivers := resultFakes()
	body := `{"outcome":"DELIVERED","location":{"latitude":-1.5}}`

	rec := do(stopRouter(orders, drivers), http.MethodPost, resultPath, body, driverStopHeaders(t))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "location.latitude and location.longitude are required")
	assert.Nil(t, drivers.registerReq, "sin ubicación completa no se registra nada")
}
