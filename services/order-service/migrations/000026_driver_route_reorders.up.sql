CREATE TABLE driver_route_reorders (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    route_id TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    reason VARCHAR(300) NOT NULL,
    previous_order UUID[] NOT NULL,
    new_order UUID[] NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
