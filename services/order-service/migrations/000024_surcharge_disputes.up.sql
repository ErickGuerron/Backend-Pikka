-- A customer's objection to one surcharge (ADR 0007, section 6). While a dispute is OPEN the overdue sweep does not
-- count that surcharge, so the account is not blocked for it (ADR 0006, section 5).
--
-- Decisions:
--   * UNIQUE (surcharge_id): one dispute per surcharge, so a double click or a retry cannot open two.
--   * The outcome is the status: IN_FAVOR waives the surcharge, AGAINST leaves it as it was.
--   * The audit columns are set together or not at all, and exactly when the dispute is no longer OPEN.
--   * No foreign key to orders: surcharges does not have one either, and the migration reversibility test drops orders.
CREATE TABLE surcharge_disputes (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    surcharge_id      UUID NOT NULL REFERENCES surcharges (id),
    customer_id       UUID NOT NULL REFERENCES customers (id),
    reason            VARCHAR(300) NOT NULL,
    status            VARCHAR(12) NOT NULL DEFAULT 'OPEN',
    opened_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_by       UUID,
    resolution_reason VARCHAR(300),
    resolved_at       TIMESTAMPTZ,
    CONSTRAINT surcharge_disputes_status_valid CHECK (status IN ('OPEN', 'IN_FAVOR', 'AGAINST')),
    CONSTRAINT surcharge_disputes_reason_not_blank CHECK (btrim(reason) <> ''),
    CONSTRAINT surcharge_disputes_one_per_surcharge UNIQUE (surcharge_id),
    CONSTRAINT surcharge_disputes_resolution_audit CHECK (
        (status = 'OPEN') = (resolved_by IS NULL)
        AND (resolved_by IS NULL) = (resolution_reason IS NULL)
        AND (resolved_by IS NULL) = (resolved_at IS NULL)
    )
);

-- The staff's queue reads only the open rows, oldest first.
CREATE INDEX surcharge_disputes_open_idx ON surcharge_disputes (opened_at) WHERE status = 'OPEN';
