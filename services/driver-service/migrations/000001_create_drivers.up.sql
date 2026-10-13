-- Run with driver_service_user and search_path=drivers,public.
-- No foreign keys: user_id references the Auth Service schema only by value.
CREATE TABLE drivers (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL,
    full_name  VARCHAR(120) NOT NULL,
    phone      VARCHAR(20) NOT NULL,
    active     BOOLEAN NOT NULL DEFAULT TRUE,
    status     VARCHAR(20) NOT NULL DEFAULT 'AVAILABLE',
    route_id   TEXT NOT NULL DEFAULT '',
    version    BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT drivers_full_name_valid CHECK (char_length(btrim(full_name)) BETWEEN 1 AND 120),
    CONSTRAINT drivers_status_valid CHECK (status IN ('AVAILABLE', 'ASSIGNED', 'ON_ROUTE', 'OFFLINE')),
    -- Solo un repartidor en ASSIGNED u ON_ROUTE tiene ruta reservada.
    CONSTRAINT drivers_route_matches_status CHECK (
        (status IN ('ASSIGNED', 'ON_ROUTE')) = (route_id <> '')
    ),
    -- Un repartidor inactivo nunca queda AVAILABLE (regla del dominio).
    CONSTRAINT drivers_inactive_not_available CHECK (active OR status <> 'AVAILABLE')
);

CREATE INDEX drivers_available_idx ON drivers (id) WHERE active AND status = 'AVAILABLE';
