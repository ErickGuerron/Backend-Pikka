-- Ledger of simulated charge attempts (ADR 0006, section 2). Every attempt is stored, approved or not.
--
-- Decisions:
--   * amount NUMERIC(12,2), always positive. A refund is also a row, with its own reference.
--   * status only says SIMULATED_*: no real gateway exists in the MVP.
--   * No column for card number, expiry or CVV, and none must be added (ADR 0006, section 3).
--   * reference is unique, so the same attempt cannot be recorded twice.
CREATE TABLE payment_attempts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES customers (id),
    amount      NUMERIC(12,2) NOT NULL,
    method      VARCHAR(8)  NOT NULL,
    status      VARCHAR(20) NOT NULL,
    reference   VARCHAR(80) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT payment_attempts_amount_positive CHECK (amount > 0),
    CONSTRAINT payment_attempts_method_valid CHECK (method IN ('CASH', 'CARD')),
    CONSTRAINT payment_attempts_status_valid CHECK (status IN ('SIMULATED_APPROVED', 'SIMULATED_DECLINED')),
    CONSTRAINT payment_attempts_reference_not_blank CHECK (btrim(reference) <> '')
);

CREATE UNIQUE INDEX payment_attempts_reference_key ON payment_attempts (reference);
CREATE INDEX payment_attempts_customer_idx ON payment_attempts (customer_id, created_at DESC);
