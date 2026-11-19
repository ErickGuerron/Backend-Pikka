-- Run with driver_service_user and search_path=drivers,public.
-- Un registro de entrega por parada y ruta. La verificación de llegada se guarda junto
-- al registro para que el motor de penalizaciones la lea sin recalcularla.
CREATE TABLE deliveries (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    route_id               TEXT NOT NULL,
    stop_id                TEXT NOT NULL,
    outcome                VARCHAR(20) NOT NULL,
    reason                 VARCHAR(30) NOT NULL DEFAULT '',
    recorded_at            TIMESTAMPTZ NOT NULL,
    latitude               DOUBLE PRECISION NOT NULL,
    longitude              DOUBLE PRECISION NOT NULL,
    photo_url              TEXT NOT NULL DEFAULT '',
    distance_meters        DOUBLE PRECISION NOT NULL,
    within_radius          BOOLEAN NOT NULL,
    within_window          BOOLEAN NOT NULL,
    CONSTRAINT deliveries_route_stop_unique UNIQUE (route_id, stop_id),
    CONSTRAINT deliveries_stop_valid CHECK (btrim(stop_id) <> '' AND btrim(route_id) <> ''),
    CONSTRAINT deliveries_outcome_valid CHECK (outcome IN ('DELIVERED', 'NOT_DELIVERED')),
    CONSTRAINT deliveries_reason_valid CHECK (
        (outcome = 'DELIVERED' AND reason = '')
        OR (outcome = 'NOT_DELIVERED' AND reason IN (
            'CLIENT_REFUSED', 'CLIENT_ABSENT', 'OUT_OF_WINDOW',
            'PRODUCT_DAMAGED', 'WRONG_ADDRESS', 'DUPLICATE_ORDER'))
    ),
    CONSTRAINT deliveries_location_valid CHECK (
        latitude BETWEEN -90 AND 90 AND longitude BETWEEN -180 AND 180
    ),
    CONSTRAINT deliveries_distance_valid CHECK (distance_meters >= 0)
);
