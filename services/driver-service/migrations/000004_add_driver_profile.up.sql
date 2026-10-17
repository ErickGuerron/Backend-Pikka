-- Run with driver_service_user and search_path=drivers,public.
-- Datos de perfil del repartidor. Las filas anteriores reciben valores provisionales derivados
-- del nombre y del id, que hay que corregir; las filas nuevas siempre traen los datos reales.
ALTER TABLE drivers
    ADD COLUMN first_name   VARCHAR(50),
    ADD COLUMN last_name    VARCHAR(50),
    ADD COLUMN birth_date   DATE,
    ADD COLUMN vehicle_type VARCHAR(20),
    ADD COLUMN identifier   VARCHAR(20),
    ADD COLUMN plate        VARCHAR(8),
    ADD COLUMN city         VARCHAR(80);

UPDATE drivers SET
    first_name   = left(split_part(btrim(full_name), ' ', 1), 50),
    last_name    = left(coalesce(nullif(btrim(regexp_replace(btrim(full_name), '^\S+\s*', '')), ''), '-'), 50),
    birth_date   = DATE '2000-01-01',
    vehicle_type = 'MOTORIZADO',
    identifier   = 'PEND' || upper(left(replace(id::text, '-', ''), 8)),
    plate        = 'P' || upper(left(replace(id::text, '-', ''), 6)),
    city         = 'Sin ciudad';

ALTER TABLE drivers
    ALTER COLUMN first_name   SET NOT NULL,
    ALTER COLUMN last_name    SET NOT NULL,
    ALTER COLUMN birth_date   SET NOT NULL,
    ALTER COLUMN vehicle_type SET NOT NULL,
    ALTER COLUMN identifier   SET NOT NULL,
    ALTER COLUMN plate        SET NOT NULL,
    ALTER COLUMN city         SET NOT NULL,
    ADD CONSTRAINT drivers_first_name_valid CHECK (char_length(btrim(first_name)) BETWEEN 1 AND 50),
    ADD CONSTRAINT drivers_last_name_valid CHECK (char_length(btrim(last_name)) BETWEEN 1 AND 50),
    ADD CONSTRAINT drivers_city_valid CHECK (char_length(btrim(city)) BETWEEN 1 AND 80),
    ADD CONSTRAINT drivers_vehicle_type_valid CHECK (vehicle_type IN ('MOTORIZADO', 'AUTOMOVIL', 'SUV')),
    ADD CONSTRAINT drivers_identifier_valid CHECK (identifier ~ '^[A-Z0-9]{5,20}$'),
    ADD CONSTRAINT drivers_plate_valid CHECK (plate ~ '^[A-Z0-9]{5,8}$'),
    ADD CONSTRAINT drivers_user_id_unique UNIQUE (user_id),
    ADD CONSTRAINT drivers_identifier_unique UNIQUE (identifier),
    ADD CONSTRAINT drivers_plate_unique UNIQUE (plate);
