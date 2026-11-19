-- Cliente de prueba (cliente@pikka.com). user_id coincide con el usuario sembrado en auth-service (000003).
-- No aplicar en producción.
INSERT INTO customers (id, user_id, full_name, email) VALUES
    ('00000000-0000-4000-8000-000000000104', '00000000-0000-4000-8000-000000000004', 'Cliente Pikka', 'cliente@pikka.com')
ON CONFLICT (user_id) DO NOTHING;
