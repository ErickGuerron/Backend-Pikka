-- Audit of the operator's verification of a WRONG_ADDRESS surcharge (ADR 0006 amendment).
-- The three columns are set together or not at all.
ALTER TABLE surcharges
    ADD COLUMN resolved_by       UUID,
    ADD COLUMN resolution_reason VARCHAR(300),
    ADD COLUMN resolved_at       TIMESTAMPTZ,
    ADD CONSTRAINT surcharges_resolution_audit_complete CHECK (
        (resolved_by IS NULL) = (resolution_reason IS NULL)
        AND (resolved_by IS NULL) = (resolved_at IS NULL)
    );

-- The operator's queue reads only the waiting rows, oldest first.
CREATE INDEX surcharges_pending_verification_idx
    ON surcharges (created_at)
    WHERE status = 'PENDING_VERIFICATION';
