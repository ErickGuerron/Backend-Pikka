DROP INDEX IF EXISTS orders_customer_created_idx;

ALTER TABLE orders
    DROP COLUMN IF EXISTS zone_id,
    DROP COLUMN IF EXISTS customer_id;
