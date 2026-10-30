package domain

// NotDeliveredReason is the closed reason a driver gives when a stop is NOT_DELIVERED (ADR 0007,
// section 4). Driver Service validates the list; this side only decides what each reason costs.
type NotDeliveredReason string

// ChargesSurcharge reports whether the reason is the customer's fault and so triggers the automatic
// surcharge of ADR 0006 (ADR 0007, section 5). Anything else, including an unknown value, does not.
func (r NotDeliveredReason) ChargesSurcharge() bool {
	switch r {
	case "CLIENT_REFUSED", "CLIENT_ABSENT", "OUT_OF_WINDOW":
		return true
	}
	return false
}

// NeedsVerification reports whether the surcharge waits for an operator instead of being automatic:
// WRONG_ADDRESS stays PENDING_VERIFICATION (ADR 0006 amendment).
func (r NotDeliveredReason) NeedsVerification() bool {
	return r == "WRONG_ADDRESS"
}
