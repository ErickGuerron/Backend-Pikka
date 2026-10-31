package http

import (
	"net/http"

	"github.com/labstack/echo/v4"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
)

// disputeDTO is a dispute as the API shows it. The resolution fields are empty while it is open.
type disputeDTO struct {
	ID               string `json:"id"`
	SurchargeID      string `json:"surchargeId"`
	CustomerID       string `json:"customerId"`
	Reason           string `json:"reason"`
	Status           string `json:"status"`
	OpenedAt         string `json:"openedAt"`
	ResolvedBy       string `json:"resolvedBy,omitempty"`
	ResolutionReason string `json:"resolutionReason,omitempty"`
	ResolvedAt       string `json:"resolvedAt,omitempty"`
}

func toDisputeDTO(d *ordersv1.Dispute) disputeDTO {
	return disputeDTO{
		ID: d.GetId(), SurchargeID: d.GetSurchargeId(), CustomerID: d.GetCustomerId(), Reason: d.GetReason(), Status: d.GetStatus(),
		OpenedAt: d.GetOpenedAt(), ResolvedBy: d.GetResolvedBy(), ResolutionReason: d.GetResolutionReason(), ResolvedAt: d.GetResolvedAt(),
	}
}

type openDisputeRequest struct {
	Reason string `json:"reason"`
}

// openDispute forwards the customer's dispute. Only the reason comes from the body: the customer is the JWT's, so nobody
// can dispute in another customer's name.
func (h *surchargeHandlers) openDispute(c echo.Context) error {
	var req openDisputeRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	res, err := h.surcharges.OpenDispute(c.Request().Context(), callMeta(c), &ordersv1.OpenDisputeRequest{
		SurchargeId: c.Param("id"), Reason: req.Reason,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, toDisputeDTO(res.GetDispute()))
}

// listOpenDisputes answers a plain array, oldest first, like the other staff queues.
func (h *surchargeHandlers) listOpenDisputes(c echo.Context) error {
	res, err := h.surcharges.ListOpenDisputes(c.Request().Context(), callMeta(c), &ordersv1.ListOpenDisputesRequest{})
	if err != nil {
		return err
	}
	out := make([]disputeDTO, 0, len(res.GetDisputes()))
	for _, d := range res.GetDisputes() {
		out = append(out, toDisputeDTO(d))
	}
	return c.JSON(http.StatusOK, out)
}

type resolveDisputeRequest struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// resolveDispute forwards the staff's decision. The audited operator is the JWT's, never the body's.
func (h *surchargeHandlers) resolveDispute(c echo.Context) error {
	var req resolveDisputeRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	res, err := h.surcharges.ResolveDispute(c.Request().Context(), callMeta(c), &ordersv1.ResolveDisputeRequest{
		Id: c.Param("id"), Decision: req.Decision, Reason: req.Reason,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, toDisputeDTO(res.GetDispute()))
}
