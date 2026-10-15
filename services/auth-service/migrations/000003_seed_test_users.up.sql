-- Usuarios de prueba para desarrollo y pruebas de integración. Contraseña común: Password123.
-- Los UUID son fijos: order-service (customers) y driver-service (drivers) los referencian por valor.
-- No aplicar en producción: estas credenciales son públicas en el repositorio.
INSERT INTO users (id, email, full_name, password_hash, role) VALUES
    ('00000000-0000-4000-8000-000000000001', 'admin@pikka.com',     'Admin Pikka',     '$2b$12$Xe6KpOyHunOXSgVhvIalFOFNdxk19xkB9oXMj0MtsgWM.QTuTMAVK', 'ADMIN'),
    ('00000000-0000-4000-8000-000000000002', 'operador@pikka.com',  'Operador Pikka',  '$2b$12$Xe6KpOyHunOXSgVhvIalFOFNdxk19xkB9oXMj0MtsgWM.QTuTMAVK', 'OPERATOR'),
    ('00000000-0000-4000-8000-000000000003', 'repartidor@pikka.com', 'Repartidor Pikka', '$2b$12$Xe6KpOyHunOXSgVhvIalFOFNdxk19xkB9oXMj0MtsgWM.QTuTMAVK', 'DRIVER'),
    ('00000000-0000-4000-8000-000000000004', 'cliente@pikka.com',  'Cliente Pikka',   '$2b$12$Xe6KpOyHunOXSgVhvIalFOFNdxk19xkB9oXMj0MtsgWM.QTuTMAVK', 'CUSTOMER')
ON CONFLICT (email) DO NOTHING;
