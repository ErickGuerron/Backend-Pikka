-- Reason a stop was closed NOT_DELIVERED when that reason creates a surcharge incident (ADR 0006). It is written in the
-- same transaction as the new order status, so a surcharge that failed to record when the stop closed can be
-- recovered later by the reconciliation sweep: Order Service does not otherwise keep the reason (it lives in Driver
-- Service). Orders closed before this table existed have no row and are not reconciled.
CREATE TABLE order_penalty_reasons (
    order_id    UUID PRIMARY KEY REFERENCES orders (id),
    reason      VARCHAR(32) NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT order_penalty_reasons_reason_not_blank CHECK (btrim(reason) <> '')
);

-- The sweep reads the oldest rows first.
CREATE INDEX order_penalty_reasons_recorded_idx ON order_penalty_reasons (recorded_at, order_id);
