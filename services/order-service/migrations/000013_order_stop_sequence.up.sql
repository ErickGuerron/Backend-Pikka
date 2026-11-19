-- Position of the order in its route (1 is the first stop). NULL means unassigned: orders assigned
-- through the legacy order_ids form, and every order created before this migration.
ALTER TABLE orders ADD COLUMN stop_sequence INTEGER CHECK (stop_sequence IS NULL OR stop_sequence >= 1);

-- A route cannot have two orders in the same position. The partial index ignores unassigned orders.
CREATE UNIQUE INDEX orders_route_stop_sequence_key
    ON orders (route_id, stop_sequence)
    WHERE stop_sequence IS NOT NULL;
