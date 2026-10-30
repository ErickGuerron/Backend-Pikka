package http

import (
	"net/http"

	"github.com/labstack/echo/v4"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
)

type setReassignmentFeeRequest struct {
	Amount string `json:"amount"`
	Reason string `json:"reason"`
}

type reassignmentFeeDTO struct {
	ID        string `json:"id"`
	Amount    string `json:"amount"`
	Reason    string `json:"reason"`
	SetBy     string `json:"setBy"`
	CreatedAt string `json:"createdAt"`
}

func feeDTO(f *ordersv1.ReassignmentFee) reassignmentFeeDTO {
	return reassignmentFeeDTO{ID: f.GetId(), Amount: f.GetAmount(), Reason: f.GetReason(), SetBy: f.GetSetBy(), CreatedAt: f.GetCreatedAt()}
}

// setReassignmentFee defines the fixed reassignment fee. The route is ADMIN only and Order Service checks the
// role again; the audited administrator is the JWT's, never the body's.
func (h *surchargeHandlers) setReassignmentFee(c echo.Context) error {
	var req setReassignmentFeeRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	res, err := h.surcharges.SetReassignmentFee(c.Request().Context(), callMeta(c), &ordersv1.SetReassignmentFeeRequest{
		Amount: req.Amount, Reason: req.Reason,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, feeDTO(res.GetFee()))
}

// getReassignmentFee shows the current fee to the staff.
func (h *surchargeHandlers) getReassignmentFee(c echo.Context) error {
	res, err := h.surcharges.GetReassignmentFee(c.Request().Context(), callMeta(c), &ordersv1.GetReassignmentFeeRequest{})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, feeDTO(res.GetFee()))
}

type applyReassignmentChargeRequest struct {
	Reason string `json:"reason"`
}

type reassignmentChargeDTO struct {
	ID         string `json:"id"`
	CustomerID string `json:"customerId"`
	OrderID    string `json:"orderId"`
	Amount     string `json:"amount"`
	Reason     string `json:"reason"`
	AppliedBy  string `json:"appliedBy"`
	CreatedAt  string `json:"createdAt"`
}

// applyReassignmentCharge charges the current fixed fee for an order. It reads no amount: nothing the client sends
// can change what is charged.
func (h *surchargeHandlers) applyReassignmentCharge(c echo.Context) error {
	var req applyReassignmentChargeRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	res, err := h.surcharges.ApplyReassignmentCharge(c.Request().Context(), callMeta(c), &ordersv1.ApplyReassignmentChargeRequest{
		OrderId: c.Param("id"), Reason: req.Reason,
	})
	if err != nil {
		return err
	}
	ch := res.GetCharge()
	return c.JSON(http.StatusCreated, reassignmentChargeDTO{
		ID: ch.GetId(), CustomerID: ch.GetCustomerId(), OrderID: ch.GetOrderId(), Amount: ch.GetAmount(),
		Reason: ch.GetReason(), AppliedBy: ch.GetAppliedBy(), CreatedAt: ch.GetCreatedAt(),
	})
}
