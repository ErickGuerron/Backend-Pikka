package http

import (
	"context"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	customerv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/customer/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

// CustomerQueryClient es lo que los endpoints de consulta de clientes
// necesitan del Order Service.
type CustomerQueryClient interface {
	GetByID(ctx context.Context, m grpcclient.CallMeta, req *customerv1.GetByIDRequest) (*customerv1.GetByIDResponse, error)
	ListCustomers(ctx context.Context, m grpcclient.CallMeta, req *customerv1.ListCustomersRequest) (*customerv1.ListCustomersResponse, error)
	UpdateStatus(ctx context.Context, m grpcclient.CallMeta, req *customerv1.UpdateStatusRequest) (*customerv1.UpdateStatusResponse, error)
}

type setCustomerStatusRequest struct {
	Status string `json:"status"`
}

// customerHandlers traduce REST <-> gRPC para consultas de clientes. La
// autorizacion (ADMIN) la aplica el router; las reglas viven en order-service.
type customerHandlers struct {
	customers CustomerQueryClient
}

type customerDTO struct {
	ID               string `json:"id"`
	UserID           string `json:"userId"`
	FullName         string `json:"fullName"`
	Email            string `json:"email"`
	Status           string `json:"status"`
	PendingSurcharge string `json:"pendingSurcharge"`
	CreatedAt        string `json:"createdAt"`
	UpdatedAt        string `json:"updatedAt"`
	// RecidivistAt is present only for a customer marked as recidivist (ADR 0006).
	RecidivistAt string `json:"recidivistAt,omitempty"`
}

type listCustomersResponse struct {
	Customers []customerDTO `json:"customers"`
	Total     int32         `json:"total"`
	Limit     int32         `json:"limit"`
	Offset    int32         `json:"offset"`
}

func toCustomerDTO(c *customerv1.Customer) customerDTO {
	return customerDTO{
		ID:               c.GetId(),
		UserID:           c.GetUserId(),
		FullName:         c.GetFullName(),
		Email:            c.GetEmail(),
		Status:           c.GetStatus(),
		PendingSurcharge: c.GetPendingSurcharge(),
		CreatedAt:        c.GetCreatedAt(),
		UpdatedAt:        c.GetUpdatedAt(),
		RecidivistAt:     c.GetRecidivistAt(),
	}
}

func (h *customerHandlers) setStatus(c echo.Context) error {
	var req setCustomerStatusRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	res, err := h.customers.UpdateStatus(c.Request().Context(), callMeta(c), &customerv1.UpdateStatusRequest{
		Id:     c.Param("id"),
		Status: req.Status,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, toCustomerDTO(res.GetCustomer()))
}

func (h *customerHandlers) get(c echo.Context) error {
	res, err := h.customers.GetByID(c.Request().Context(), callMeta(c), &customerv1.GetByIDRequest{Id: c.Param("id")})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, toCustomerDTO(res.GetCustomer()))
}

func (h *customerHandlers) list(c echo.Context) error {
	limit, err := intQuery(c, "limit")
	if err != nil {
		return err
	}
	offset, err := intQuery(c, "offset")
	if err != nil {
		return err
	}
	res, err := h.customers.ListCustomers(c.Request().Context(), callMeta(c), &customerv1.ListCustomersRequest{
		Limit:  limit,
		Offset: offset,
		Status: c.QueryParam("status"),
	})
	if err != nil {
		return err
	}
	out := listCustomersResponse{
		Customers: make([]customerDTO, 0, len(res.GetCustomers())),
		Total:     res.GetTotal(),
		Limit:     limit,
		Offset:    offset,
	}
	for _, cu := range res.GetCustomers() {
		out.Customers = append(out.Customers, toCustomerDTO(cu))
	}
	return c.JSON(http.StatusOK, out)
}

// intQuery lee un entero opcional; ausente => 0 (default del servicio).
// Un valor no numerico es 400; los negativos los rechaza el servicio.
func intQuery(c echo.Context, name string) (int32, error) {
	raw := c.QueryParam(name)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, echo.NewHTTPError(http.StatusBadRequest, name+" must be an integer")
	}
	return int32(n), nil
}
