-- Penalty incidences (ADR 0006, section 1). One row per undelivered order caused by the customer.
--
-- Decisions:
--   * order_id is UNIQUE: a surcharge is charged once per incidence and never accumulates.
--   * amount is NULL until the 15 % can be computed over the customer's next order; it is positive when set.
--   * No FK to orders: the order may be archived or moved without losing the penalty trail.
CREATE TABLE surcharges (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES customers (id),
    order_id    UUID NOT NULL,
    reason      VARCHAR(300) NOT NULL,
    status      VARCHAR(24)  NOT NULL DEFAULT 'PENDING',
    amount      NUMERIC(12,2),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT surcharges_status_valid
        CHECK (status IN ('PENDING', 'PENDING_VERIFICATION', 'APPLIED', 'PAID', 'WAIVED')),
    CONSTRAINT surcharges_amount_positive CHECK (amount IS NULL OR amount > 0),
    CONSTRAINT surcharges_reason_not_blank CHECK (btrim(reason) <> '')
);

CREATE UNIQUE INDEX surcharges_order_id_key ON surcharges (order_id);
CREATE INDEX surcharges_customer_status_idx ON surcharges (customer_id, status);
