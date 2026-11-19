-- Esquema zones (search_path del usuario zone_service_user).
-- PostGIS ya está instalado en public por el init de PostgreSQL.
CREATE TABLE zones (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code       VARCHAR(32)  NOT NULL,
    name       VARCHAR(120) NOT NULL,
    area       public.geography(POLYGON, 4326) NOT NULL,
    version    INTEGER      NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT zones_code_not_blank CHECK (btrim(code) <> ''),
    CONSTRAINT zones_name_not_blank CHECK (btrim(name) <> ''),
    CONSTRAINT zones_version_positive CHECK (version > 0)
);

CREATE UNIQUE INDEX zones_code_key ON zones (code);

-- Índice espacial: acelera ST_Covers / ST_DWithin en la clasificación de puntos.
CREATE INDEX zones_area_gist ON zones USING GIST (area);
