UPDATE drivers SET
    full_name    = 'Repartidor Pikka',
    first_name   = 'Repartidor',
    last_name    = 'Pikka',
    birth_date   = DATE '2000-01-01',
    phone        = '0999999999',
    vehicle_type = 'MOTORIZADO',
    identifier   = 'PEND00000203',
    plate        = 'P000203',
    city         = 'Sin ciudad',
    updated_at   = now()
WHERE id = '00000000-0000-4000-8000-000000000203';
