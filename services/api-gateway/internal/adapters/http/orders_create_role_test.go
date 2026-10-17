package http_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Solo el cliente registra pedidos (ADR 0007): el personal los revisa, no los crea. El gateway corta
// antes de validar el cuerpo ni llamar al Order Service, así que la respuesta no depende del contenido.
func TestCreateOrder_OnlyCustomersPass(t *testing.T) {
	for _, role := range []string{"ADMIN", "OPERATOR", "DRIVER"} {
		for name, body := range map[string]string{"cuerpo válido": validOrderBody, "cuerpo vacío": `{}`} {
			fake := &fakeOrderClient{}

			rec := do(ordersRouter(fake), http.MethodPost, "/api/v1/orders", body, authHdr(t, role))

			assert.Equal(t, http.StatusForbidden, rec.Code, role+" "+name)
			assert.Equal(t, "FORBIDDEN", decodeError(t, rec).Code, role+" "+name)
			assert.Nil(t, fake.createReq, role+" "+name+": no debe llegar al Order Service")
		}
	}
}

func TestCreateOrder_CustomerStillReachesTheService(t *testing.T) {
	fake := &fakeOrderClient{}

	rec := do(ordersRouter(fake), http.MethodPost, "/api/v1/orders", validOrderBody, authHdr(t, "CUSTOMER"))

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.NotNil(t, fake.createReq)
}

func TestCreateOrder_WithoutTokenIs401(t *testing.T) {
	rec := do(ordersRouter(&fakeOrderClient{}), http.MethodPost, "/api/v1/orders", validOrderBody, nil)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// La compuerta solo cierra el alta: el personal sigue pudiendo listar y revisar.
func TestStaffKeepsReadingAndReviewingOrders(t *testing.T) {
	fake := &fakeOrderClient{}

	assert.Equal(t, http.StatusOK, do(ordersRouter(fake), http.MethodGet, "/api/v1/orders/exceptions", "", authHdr(t, "OPERATOR")).Code)
	assert.Equal(t, http.StatusOK, do(ordersRouter(fake), http.MethodPost, "/api/v1/orders/o-7/review",
		`{"action":"CONFIRM","reason":"Confirmado por teléfono"}`, authHdr(t, "ADMIN")).Code)
}
