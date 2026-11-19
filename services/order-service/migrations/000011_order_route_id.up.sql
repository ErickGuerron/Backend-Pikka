-- Route that carries the order once it is ROUTE_ASSIGNED. Routing ids have the form "{zone_id}#{n}",
-- so the column is text. NULL until the order is assigned.
ALTER TABLE orders ADD COLUMN route_id TEXT;
