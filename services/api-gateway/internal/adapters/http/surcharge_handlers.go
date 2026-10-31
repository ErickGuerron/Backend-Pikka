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
	ApplyRecidivismCharge(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.ApplyRecidivismChargeRequest) (*ordersv1.ApplyRecidivismChargeResponse, error)
	ClearRecidivist(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.ClearRecidivistRequest) (*ordersv1.ClearRecidivistResponse, error)
	SetReassignmentFee(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.SetReassignmentFeeRequest) (*ordersv1.SetReassignmentFeeResponse, error)
	GetReassignmentFee(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.GetReassignmentFeeRequest) (*ordersv1.GetReassignmentFeeResponse, error)
	ApplyReassignmentCharge(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.ApplyReassignmentChargeRequest) (*ordersv1.ApplyReassignmentChargeResponse, error)
	ListRecidivists(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.ListRecidivistsRequest) (*ordersv1.ListRecidivistsResponse, error)
	PayMyBalance(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.PayMyBalanceRequest) (*ordersv1.PayMyBalanceResponse, error)
	GetMyBalance(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.GetMyBalanceRequest) (*ordersv1.GetMyBalanceResponse, error)
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

// applyRecidivismChargeRequest carries the amount as a decimal string, like pendingSurcharge, so a
// JSON number (a float) is rejected as a bad body.
type applyRecidivismChargeRequest struct {
	Amount string `json:"amount"`
	Reason string `json:"reason"`
}

type recidivismChargeDTO struct {
	ID           string `json:"id"`
	CustomerID   string `json:"customerId"`
	RecidivistAt string `json:"recidivistAt"`
	Amount       string `json:"amount"`
	Reason       string `json:"reason"`
	AppliedBy    string `json:"appliedBy"`
	CreatedAt    string `json:"createdAt"`
}

// applyRecidivismCharge forwards the operator's extra charge. Order Service checks the role, that the
// customer is a recidivist and that the reincidence was not charged yet; the audited operator is the
// JWT's, never the body's.
func (h *surchargeHandlers) applyRecidivismCharge(c echo.Context) error {
	var req applyRecidivismChargeRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	res, err := h.surcharges.ApplyRecidivismCharge(c.Request().Context(), callMeta(c), &ordersv1.ApplyRecidivismChargeRequest{
		CustomerId: c.Param("id"),
		Amount:     req.Amount,
		Reason:     req.Reason,
	})
	if err != nil {
		return err
	}
	ch := res.GetCharge()
	return c.JSON(http.StatusCreated, recidivismChargeDTO{
		ID: ch.GetId(), CustomerID: ch.GetCustomerId(), RecidivistAt: ch.GetRecidivistAt(), Amount: ch.GetAmount(),
		Reason: ch.GetReason(), AppliedBy: ch.GetAppliedBy(), CreatedAt: ch.GetCreatedAt(),
	})
}

// recidivistDTO is all an operator gets about a recidivist: no email, no address.
type recidivistDTO struct {
	CustomerID       string `json:"customerId"`
	FullName         string `json:"fullName"`
	ContactPhone     string `json:"contactPhone"`
	RecidivistAt     string `json:"recidivistAt"`
	PendingSurcharge string `json:"pendingSurcharge"`
	ChargeApplied    bool   `json:"chargeApplied"`
}

// listRecidivists answers a plain array, oldest mark first, like the other operator queues.
func (h *surchargeHandlers) listRecidivists(c echo.Context) error {
	res, err := h.surcharges.ListRecidivists(c.Request().Context(), callMeta(c), &ordersv1.ListRecidivistsRequest{})
	if err != nil {
		return err
	}
	out := make([]recidivistDTO, 0, len(res.GetRecidivists()))
	for _, r := range res.GetRecidivists() {
		out = append(out, recidivistDTO{
			CustomerID: r.GetCustomerId(), FullName: r.GetFullName(), ContactPhone: r.GetContactPhone(),
			RecidivistAt: r.GetRecidivistAt(), PendingSurcharge: r.GetPendingSurcharge(), ChargeApplied: r.GetChargeApplied(),
		})
	}
	return c.JSON(http.StatusOK, out)
}

type clearRecidivistRequest struct {
	Reason string `json:"reason"`
}

type recidivistClearDTO struct {
	ID           string `json:"id"`
	CustomerID   string `json:"customerId"`
	RecidivistAt string `json:"recidivistAt"`
	Reason       string `json:"reason"`
	ClearedBy    string `json:"clearedBy"`
	CreatedAt    string `json:"createdAt"`
}

// clearRecidivist removes the customer's reincidence mark. The route is ADMIN only and Order Service checks the
// role again; the audited administrator is the JWT's, never the body's.
func (h *surchargeHandlers) clearRecidivist(c echo.Context) error {
	var req clearRecidivistRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	res, err := h.surcharges.ClearRecidivist(c.Request().Context(), callMeta(c), &ordersv1.ClearRecidivistRequest{
		CustomerId: c.Param("id"),
		Reason:     req.Reason,
	})
	if err != nil {
		return err
	}
	k := res.GetClear()
	return c.JSON(http.StatusOK, recidivistClearDTO{
		ID: k.GetId(), CustomerID: k.GetCustomerId(), RecidivistAt: k.GetRecidivistAt(), Reason: k.GetReason(),
		ClearedBy: k.GetClearedBy(), CreatedAt: k.GetCreatedAt(),
	})
}

type balancePaymentDTO struct {
	Status string `json:"status"`
	// Simulated is always true: there is no real gateway, so the interface can never show a real charge
	// (ADR 0006, section 4).
	Simulated bool   `json:"simulated"`
	Amount    string `json:"amount"`
	Reference string `json:"reference"`
	Unblocked bool   `json:"unblocked"`
	CreatedAt string `json:"createdAt"`
}

// payMyBalance pays the whole balance of the authenticated customer. It reads no body: nothing the
// client sends can change the amount. A declined simulated card answers 200 with its status.
func (h *surchargeHandlers) payMyBalance(c echo.Context) error {
	res, err := h.surcharges.PayMyBalance(c.Request().Context(), callMeta(c), &ordersv1.PayMyBalanceRequest{})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, balancePaymentDTO{
		Status: res.GetStatus(), Simulated: true, Amount: res.GetAmount(), Reference: res.GetReference(),
		Unblocked: res.GetUnblocked(), CreatedAt: res.GetCreatedAt(),
	})
}

type balanceLineDTO struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	OrderID   string `json:"orderId,omitempty"`
	Reason    string `json:"reason"`
	Amount    string `json:"amount"`
	StartedAt string `json:"startedAt"`
	DueAt     string `json:"dueAt"`
}

type myBalanceDTO struct {
	PendingSurcharge string `json:"pendingSurcharge"`
	AccountStatus    string `json:"accountStatus"`
	// Blocked is true while the account is blocked for an unpaid balance: it cannot create or cancel orders
	// until it pays (ADR 0006, section 5).
	Blocked bool `json:"blocked"`
	// Simulated is always true: payments are simulated, and the interface must say so (ADR 0006, section 4).
	Simulated bool             `json:"simulated"`
	Lines     []balanceLineDTO `json:"lines"`
}

// myBalance shows the authenticated customer what they owe. A blocked account can read it.
func (h *surchargeHandlers) myBalance(c echo.Context) error {
	res, err := h.surcharges.GetMyBalance(c.Request().Context(), callMeta(c), &ordersv1.GetMyBalanceRequest{})
	if err != nil {
		return err
	}
	lines := make([]balanceLineDTO, 0, len(res.GetLines()))
	for _, l := range res.GetLines() {
		lines = append(lines, balanceLineDTO{
			Kind: l.GetKind(), ID: l.GetId(), OrderID: l.GetOrderId(), Reason: l.GetReason(),
			Amount: l.GetAmount(), StartedAt: l.GetStartedAt(), DueAt: l.GetDueAt(),
		})
	}
	return c.JSON(http.StatusOK, myBalanceDTO{
		PendingSurcharge: res.GetPendingSurcharge(),
		AccountStatus:    res.GetAccountStatus(),
		Blocked:          res.GetAccountStatus() == "BLOQUEADO_POR_SALDO",
		Simulated:        true,
		Lines:            lines,
	})
}
