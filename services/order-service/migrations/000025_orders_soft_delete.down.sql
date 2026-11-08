DROP INDEX IF EXISTS idx_orders_active;
ALTER TABLE orders DROP COLUMN IF EXISTS deleted_at;