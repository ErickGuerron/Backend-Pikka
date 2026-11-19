-- Revierte el CHECK al estado original.
-- Asume que no hay filas con role = 'CUSTOMER' en este momento;
-- si las hay, este down falla con un CHECK violation (intencional).
ALTER TABLE users DROP CONSTRAINT users_role_valid;

ALTER TABLE users ADD CONSTRAINT users_role_valid
    CHECK (role IN ('ADMIN', 'OPERATOR', 'DRIVER'));