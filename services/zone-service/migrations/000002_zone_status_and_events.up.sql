-- Estado de la zona: una zona inactiva no clasifica puntos ni valida pedidos, pero se conserva
-- para el historial. El motivo registra por qué cambió de estado por última vez.
ALTER TABLE zones
    ADD COLUMN status TEXT NOT NULL DEFAULT 'ACTIVE',
    ADD COLUMN status_reason TEXT NOT NULL DEFAULT '',
    ADD CONSTRAINT zones_status_valid CHECK (status IN ('ACTIVE', 'INACTIVE'));

CREATE INDEX zones_status_idx ON zones (status);

-- Contador por prefijo para generar códigos legibles (NORTE-001). Incrementar la fila es atómico,
-- así que dos altas simultáneas nunca reciben el mismo número.
CREATE TABLE zone_code_sequences (
    prefix     TEXT PRIMARY KEY,
    last_value INTEGER NOT NULL DEFAULT 0 CHECK (last_value >= 0)
);

-- Bitácora de cambios de zonas. No tiene clave foránea hacia zones: el historial sobrevive
-- aunque la zona se borre físicamente.
CREATE TABLE zone_events (
    id         BIGSERIAL PRIMARY KEY,
    zone_id    UUID NOT NULL,
    action     TEXT NOT NULL CHECK (action IN ('CREATED', 'UPDATED', 'DEACTIVATED', 'REACTIVATED', 'DELETED')),
    actor_id   TEXT NOT NULL,
    reason     TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX zone_events_zone_idx ON zone_events (zone_id, created_at);