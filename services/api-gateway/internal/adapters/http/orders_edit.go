package http

import (
	"encoding/json"
	"io"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/httpx"
)

type editOrderRequest struct {
	Address          *string  `json:"address"`
	Latitude         *float64 `json:"latitude"`
	Longitude        *float64 `json:"longitude"`
	DeliveryDate     *string  `json:"deliveryDate"`
	WindowStart      *string  `json:"windowStart"`
	WindowEnd        *string  `json:"windowEnd"`
	AddressReference *string  `json:"addressReference"`
	PostalCode       *string  `json:"postalCode"`
	ContactPhone     *string  `json:"contactPhone"`
}

func (h *ordersHandlers) edit(c *httpx.Context) error {
	var req editOrderRequest
	if !isJSON(c.Request().Header.Get(httpx.HeaderContentType)) {
		return httpx.NewHTTPError(http.StatusUnsupportedMediaType, "content type must be application/json")
	}
	decoder := json.NewDecoder(c.Request().Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || decoder.Decode(new(any)) != io.EOF {
		return httpx.NewHTTPError(http.StatusBadRequest, "malformed JSON body")
	}
	res, err := h.orders.EditMyOrder(c.Request().Context(), callMeta(c), &ordersv1.EditMyOrderRequest{
		OrderId: c.Param("id"), Address: req.Address, Latitude: req.Latitude, Longitude: req.Longitude,
		DeliveryDate: req.DeliveryDate, WindowStart: req.WindowStart, WindowEnd: req.WindowEnd,
		AddressReference: req.AddressReference, PostalCode: req.PostalCode, ContactPhone: req.ContactPhone,
	})
	if status.Code(err) == codes.FailedPrecondition {
		return httpx.NewHTTPError(http.StatusUnprocessableEntity, "La nueva dirección está fuera de cobertura. Conservamos tu dirección original.")
	}
	if status.Code(err) == codes.Aborted {
		return httpx.NewHTTPError(http.StatusConflict, "El estado actual del pedido ya no permite este cambio.")
	}
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, toOrderDTO(res.GetOrder()))
}
