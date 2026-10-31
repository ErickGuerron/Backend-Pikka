-- Audit of the removal of the reincidence mark (ADR 0006, F5). Only an administrator removes it, with a reason.
-- The charges already collected (recidivism_charges) and the balance stay: only customers.recidivist_at goes.
CREATE TABLE recidivist_clears (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id   UUID NOT NULL REFERENCES customers (id),
    recidivist_at TIMESTAMPTZ NOT NULL,
    reason        VARCHAR(300) NOT NULL,
    cleared_by    UUID NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT recidivist_clears_reason_not_blank CHECK (btrim(reason) <> '')
);

CREATE INDEX recidivist_clears_customer_idx ON recidivist_clears (customer_id, created_at DESC);
