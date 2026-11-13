DROP INDEX orders_waiting_scope_fifo_idx;
ALTER TABLE orders DROP COLUMN queued_at;
