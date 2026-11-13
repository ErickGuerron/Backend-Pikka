CREATE TABLE wrong_address_reassignments (
    order_id UUID PRIMARY KEY,
    previous_route_id TEXT NOT NULL,
    previous_stop_sequence INTEGER NOT NULL,
    previous_address TEXT NOT NULL,
    previous_delivery_date DATE NOT NULL,
    travel_fee NUMERIC(12, 2) NOT NULL CHECK (travel_fee > 0),
    actor_id UUID NOT NULL,
    reason VARCHAR(300) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
