-- Extra charge an operator applies to a recidivist customer (ADR 0006, section 1). Each row is audited:
-- who applied it, why, and the amount they decided.
--
-- Decisions:
--   * UNIQUE (customer_id, recidivist_at): one charge per reincidence mark, so a double click or a
--     retry cannot charge twice. The mark is sticky, so today this means once per customer.
--   * The amount is added to customers.pending_surcharge in the same transaction (see the repository).
CREATE TABLE recidivism_charges (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id   UUID NOT NULL REFERENCES customers (id),
    recidivist_at TIMESTAMPTZ NOT NULL,
    amount        NUMERIC(12,2) NOT NULL,
    reason        VARCHAR(300) NOT NULL,
    applied_by    UUID NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT recidivism_charges_amount_positive CHECK (amount > 0),
    CONSTRAINT recidivism_charges_reason_not_blank CHECK (btrim(reason) <> ''),
    CONSTRAINT recidivism_charges_once_per_mark UNIQUE (customer_id, recidivist_at)
);
