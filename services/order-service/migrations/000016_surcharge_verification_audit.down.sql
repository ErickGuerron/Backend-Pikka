DROP INDEX IF EXISTS surcharges_pending_verification_idx;

ALTER TABLE surcharges
    DROP CONSTRAINT IF EXISTS surcharges_resolution_audit_complete,
    DROP COLUMN IF EXISTS resolved_at,
    DROP COLUMN IF EXISTS resolution_reason,
    DROP COLUMN IF EXISTS resolved_by;
