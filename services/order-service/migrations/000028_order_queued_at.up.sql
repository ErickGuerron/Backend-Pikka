ALTER TABLE orders ADD COLUMN queued_at TIMESTAMPTZ;
UPDATE orders SET queued_at = created_at;
ALTER TABLE orders ALTER COLUMN queued_at SET DEFAULT clock_timestamp(), ALTER COLUMN queued_at SET NOT NULL;

CREATE INDEX orders_waiting_scope_fifo_idx ON orders (zone_id, delivery_date, queued_at, id)
    WHERE deleted_at IS NULL AND status = 'PENDING' AND route_id IS NULL;
