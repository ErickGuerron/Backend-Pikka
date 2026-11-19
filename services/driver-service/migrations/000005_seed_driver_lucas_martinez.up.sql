-- Completa el repartidor de prueba sembrado en 000003 (repartidor@pikka.com): Lucas Martinez, 27 años.
-- Nació el 15/03/1999, así que cumple 28 el 15/03/2027. El user_id sigue siendo el de auth-service.
-- No aplicar en producción.
UPDATE drivers SET
    full_name    = 'Lucas Martinez',
    first_name   = 'Lucas',
    last_name    = 'Martinez',
    birth_date   = DATE '1999-03-15',
    phone        = '0987654321',
    vehicle_type = 'MOTORIZADO',
    identifier   = '1712345678',
    plate        = 'PBA1234',
    city         = 'Quito',
    updated_at   = now()
WHERE id = '00000000-0000-4000-8000-000000000203';
