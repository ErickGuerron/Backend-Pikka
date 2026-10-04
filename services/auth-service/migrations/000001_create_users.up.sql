CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         VARCHAR(254) NOT NULL,
    full_name     VARCHAR(120) NOT NULL,
    password_hash TEXT         NOT NULL,
    role          VARCHAR(16)  NOT NULL,
    active        BOOLEAN      NOT NULL DEFAULT TRUE,
    version       INTEGER      NOT NULL DEFAULT 1,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT users_email_lowercase CHECK (email = lower(email)),
    CONSTRAINT users_role_valid CHECK (role IN ('ADMIN', 'OPERATOR', 'DRIVER')),
    CONSTRAINT users_version_positive CHECK (version > 0)
);

CREATE UNIQUE INDEX users_email_key ON users (email);
CREATE INDEX users_role_idx ON users (role);
