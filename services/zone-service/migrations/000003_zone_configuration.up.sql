-- Configuración operativa de la zona: depósito de salida, capacidad de ruta, horario de servicio,
-- cortes de pedidos y prioridad. Solo el administrador la define (ver ADR de zonas).
--
-- Zonas anteriores a esta migración: los valores por defecto cubren el horario y la capacidad,
-- pero el depósito queda NULL a propósito. Inventar coordenadas de salida produciría rutas falsas;
-- hay que asignar el depósito antes de armar rutas para esas zonas.
ALTER TABLE zones
    ADD COLUMN description            TEXT NOT NULL DEFAULT '',
    ADD COLUMN depot_name             TEXT NOT NULL DEFAULT '',
    ADD COLUMN depot_lat              DOUBLE PRECISION,
    ADD COLUMN depot_lng              DOUBLE PRECISION,
    ADD COLUMN max_orders_per_route   INTEGER NOT NULL DEFAULT 4,
    ADD COLUMN route_capacity_kg      DOUBLE PRECISION NOT NULL DEFAULT 45,
    ADD COLUMN timezone               TEXT NOT NULL DEFAULT 'America/Guayaquil',
    ADD COLUMN service_days           TEXT[] NOT NULL DEFAULT ARRAY['MON','TUE','WED','THU','FRI','SAT','SUN'],
    ADD COLUMN delivery_window_from   TEXT NOT NULL DEFAULT '08:00',
    ADD COLUMN delivery_window_to     TEXT NOT NULL DEFAULT '20:00',
    ADD COLUMN cutoff_time            TEXT NOT NULL DEFAULT '14:00',
    ADD COLUMN dispatch_time          TEXT NOT NULL DEFAULT '15:30',
    ADD COLUMN priority               TEXT NOT NULL DEFAULT 'NORMAL',
    ADD COLUMN daily_order_limit      INTEGER NOT NULL DEFAULT 250,
    ADD COLUMN created_by             TEXT NOT NULL DEFAULT 'legacy',
    ADD COLUMN updated_by             TEXT NOT NULL DEFAULT 'legacy',
    ADD CONSTRAINT zones_description_length CHECK (char_length(description) <= 500),
    ADD CONSTRAINT zones_depot_name_not_blank CHECK (depot_name = '' OR btrim(depot_name) <> ''),
    ADD CONSTRAINT zones_depot_lat_range CHECK (depot_lat IS NULL OR depot_lat BETWEEN -90 AND 90),
    ADD CONSTRAINT zones_depot_lng_range CHECK (depot_lng IS NULL OR depot_lng BETWEEN -180 AND 180),
    ADD CONSTRAINT zones_max_orders_positive CHECK (max_orders_per_route > 0),
    ADD CONSTRAINT zones_route_capacity_positive CHECK (route_capacity_kg > 0),
    ADD CONSTRAINT zones_daily_limit_positive CHECK (daily_order_limit > 0),
    ADD CONSTRAINT zones_service_days_valid CHECK (
        cardinality(service_days) > 0
        AND service_days <@ ARRAY['MON','TUE','WED','THU','FRI','SAT','SUN']::TEXT[]
    ),
    ADD CONSTRAINT zones_window_valid CHECK (delivery_window_from < delivery_window_to),
    ADD CONSTRAINT zones_times_format CHECK (
        delivery_window_from ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
        AND delivery_window_to ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
        AND cutoff_time ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
        AND dispatch_time ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
    ),
    ADD CONSTRAINT zones_priority_valid CHECK (priority IN ('NORMAL', 'HIGH', 'LOW'));
