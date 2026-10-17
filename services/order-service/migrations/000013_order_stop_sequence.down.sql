DROP INDEX IF EXISTS orders_route_stop_sequence_key;

ALTER TABLE orders DROP COLUMN IF EXISTS stop_sequence;
