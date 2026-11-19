-- Ties each order to the customer who registered it (ADR 0007) and to the zone
-- that covered its address when it was validated (ADR 0007, section 3).
-- Both references stay inside the orders schema: no cross-schema foreign keys.
-- Orders created before this migration keep NULL in both columns; new orders always set customer_id.
ALTER TABLE orders
    ADD COLUMN customer_id UUID REFERENCES customers (id) ON DELETE RESTRICT,
    ADD COLUMN zone_id     UUID;

-- Customer history: "mis pedidos" sorted by registration instant.
CREATE INDEX orders_customer_created_idx ON orders (customer_id, created_at DESC, id);
