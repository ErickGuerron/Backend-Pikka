-- BE-T4b: tabla customers del esquema orders.
--
-- Decisiones:
--   * UUID PK con gen_random_uuid() (se requiere pgcrypto).
--   * user_id: referencia lógica al esquema auth (ADR 0003). Sin FK
--     (el esquema orders no tiene permisos sobre auth), se garantiza
--     unicidad con UNIQUE INDEX.
--   * pending_surcharge NUMERIC(12,2): nunca float en Go, se mapea a
--     shopspring/decimal.Decimal.
--   * CHECK constraints para mantener la integridad a nivel de DB.

CREATE TABLE customers (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL,
    full_name         VARCHAR(120) NOT NULL,
    email             VARCHAR(254) NOT NULL,
    status            VARCHAR(32)  NOT NULL DEFAULT 'ACTIVO',
    pending_surcharge NUMERIC(12,2) NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT customers_status_valid
        CHECK (status IN ('ACTIVO', 'BLOQUEADO_POR_SALDO', 'BLOQUEADO_POR_INCIDENCIAS')),
    CONSTRAINT customers_email_lowercase
        CHECK (email = lower(email)),
    CONSTRAINT customers_pending_surcharge_nonneg
        CHECK (pending_surcharge >= 0)
);

-- Unicidad de la referencia lógica a auth.users(id) (ADR 0003).
CREATE UNIQUE INDEX customers_user_id_key ON customers (user_id);

-- Búsquedas operativas: "clientes bloqueados" y búsqueda por email.
CREATE INDEX customers_status_idx ON customers (status);
CREATE INDEX customers_email_key ON customers (email);
