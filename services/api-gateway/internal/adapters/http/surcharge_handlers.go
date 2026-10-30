package http

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

// SurchargeClient is the part of Order Service the operator uses to verify surcharges (ADR 0006
// amendment). It is separate from OrderClient so the order routes keep their own fakes.
type SurchargeClient interface {
	ListSurchargesPendingVerification(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.ListSurchargesPendingVerificationRequest) (*ordersv1.ListSurchargesPendingVerificationResponse, error)
	ResolveSurchargeVerification(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.ResolveSurchargeVerificationRequest) (*ordersv1.ResolveSurchargeVerificationResponse, error)
}

type surchargeHandlers struct {
	surcharges SurchargeClient
}

type resolveSurchargeRequest struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

type surchargeDTO struct {
	ID               string `json:"id"`
	CustomerID       string `json:"customerId"`
	OrderID          string `json:"orderId"`
	Reason           string `json:"reason"`
	Status           string `json:"status"`
	Amount           string `json:"amount,omitempty"`
	CreatedAt        string `json:"createdAt"`
	ResolvedBy       string `json:"resolvedBy,omitempty"`
	ResolutionReason string `json:"resolutionReason,omitempty"`
	ResolvedAt       string `json:"resolvedAt,omitempty"`
}

func toSurchargeDTO(s *ordersv1.Surcharge) surchargeDTO {
	return surchargeDTO{
		ID: s.GetId(), CustomerID: s.GetCustomerId(), OrderID: s.GetOrderId(), Reason: s.GetReason(),
		Status: s.GetStatus(), Amount: s.GetAmount(), CreatedAt: s.GetCreatedAt(),
		ResolvedBy: s.GetResolvedBy(), ResolutionReason: s.GetResolutionReason(), ResolvedAt: s.GetResolvedAt(),
	}
}

// listPendingVerification answers a plain array, like the other operator queues.
func (h *surchargeHandlers) listPendingVerification(c echo.Context) error {
	res, err := h.surcharges.ListSurchargesPendingVerification(c.Request().Context(), callMeta(c), &ordersv1.ListSurchargesPendingVerificationRequest{})
	if err != nil {
		return err
	}
	out := make([]surchargeDTO, 0, len(res.GetSurcharges()))
	for _, s := range res.GetSurcharges() {
		out = append(out, toSurchargeDTO(s))
	}
	return c.JSON(http.StatusOK, out)
}

// resolve forwards the operator's decision. Order Service validates the decision, the reason and the
// state; the audited operator id comes from the verified JWT, never from the body.
func (h *surchargeHandlers) resolve(c echo.Context) error {
	var req resolveSurchargeRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	res, err := h.surcharges.ResolveSurchargeVerification(c.Request().Context(), callMeta(c), &ordersv1.ResolveSurchargeVerificationRequest{
		Id:       c.Param("id"),
		Decision: req.Decision,
		Reason:   req.Reason,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, toSurchargeDTO(res.GetSurcharge()))
}
