-- Fixed fee for reassigning an order (ADR 0006, wrong address), set by an administrator with an audited reason.
-- Every change is a new row; the latest is the current fee. With no row there is no fee: nothing is charged.
CREATE TABLE reassignment_fees (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    amount     NUMERIC(12,2) NOT NULL,
    reason     VARCHAR(300) NOT NULL,
    set_by     UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT reassignment_fees_amount_positive CHECK (amount > 0),
    CONSTRAINT reassignment_fees_reason_not_blank CHECK (btrim(reason) <> '')
);

-- One charge per order, so a double click or a retry cannot charge twice. paid_at works like in
-- recidivism_charges: NULL means unpaid, and the overdue sweep reads it.
CREATE TABLE reassignment_charges (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES customers (id),
    order_id    UUID NOT NULL REFERENCES orders (id),
    amount      NUMERIC(12,2) NOT NULL,
    reason      VARCHAR(300) NOT NULL,
    applied_by  UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at     TIMESTAMPTZ,
    CONSTRAINT reassignment_charges_amount_positive CHECK (amount > 0),
    CONSTRAINT reassignment_charges_reason_not_blank CHECK (btrim(reason) <> ''),
    CONSTRAINT reassignment_charges_once_per_order UNIQUE (order_id)
);

CREATE INDEX reassignment_charges_customer_unpaid_idx ON reassignment_charges (customer_id) WHERE paid_at IS NULL;
