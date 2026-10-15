-- Repartidor de prueba (repartidor@pikka.com). user_id coincide con el usuario sembrado en auth-service (000003).
-- No aplicar en producción.
INSERT INTO drivers (id, user_id, full_name, phone) VALUES
    ('00000000-0000-4000-8000-000000000203', '00000000-0000-4000-8000-000000000003', 'Repartidor Pikka', '0999999999')
ON CONFLICT DO NOTHING;
