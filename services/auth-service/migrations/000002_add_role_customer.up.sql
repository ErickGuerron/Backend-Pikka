-- Agrega 'CUSTOMER' al conjunto de roles válidos.
-- Soltar y recrear el CHECK constraint es la forma soportada por PostgreSQL
-- para ampliar un enum simulado con VARCHAR + CHECK.
ALTER TABLE users DROP CONSTRAINT users_role_valid;

ALTER TABLE users ADD CONSTRAINT users_role_valid
    CHECK (role IN ('ADMIN', 'OPERATOR', 'DRIVER', 'CUSTOMER'));