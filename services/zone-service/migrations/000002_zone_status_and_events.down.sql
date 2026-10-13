DROP TABLE IF EXISTS zone_events;
DROP TABLE IF EXISTS zone_code_sequences;
ALTER TABLE zones
    DROP CONSTRAINT IF EXISTS zones_status_valid,
    DROP COLUMN IF EXISTS status_reason,
    DROP COLUMN IF EXISTS status;