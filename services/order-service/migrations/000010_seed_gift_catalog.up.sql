-- Catálogo inicial de la tienda de regalos (ADR 0009). Precios en USD.
-- stock_quantity nulo: el producto no controla stock. Los que sí lo controlan
-- arrancan con un movimiento RESTOCK inicial, para que el historial cuadre desde el inicio.
INSERT INTO products (id, name, description, unit_price, active, stock_quantity) VALUES
    ('00000000-0000-4000-8000-000000000301', 'Ramo de rosas rojas', 'Docena de rosas rojas con follaje y papel de regalo.', 28.00, TRUE, 20),
    ('00000000-0000-4000-8000-000000000302', 'Ramo de tulipanes mixtos', 'Tulipanes de colores de temporada en ramo sencillo.', 24.00, TRUE, 15),
    ('00000000-0000-4000-8000-000000000303', 'Arreglo floral en caja', 'Arreglo floral de temporada en caja de regalo con tarjeta.', 35.00, TRUE, 10),
    ('00000000-0000-4000-8000-000000000304', 'Girasoles en florero de cristal', 'Tres girasoles en florero de vidrio, listos para regalar.', 26.00, TRUE, 9),
    ('00000000-0000-4000-8000-000000000305', 'Ramo de frutas con chocolates', 'Frutas frescas de temporada con chocolates finos en ramo decorado.', 32.00, TRUE, 12),
    ('00000000-0000-4000-8000-000000000306', 'Canasta de frutas frescas', 'Canasta con frutas de temporada seleccionadas el mismo día.', 29.00, TRUE, 8),
    ('00000000-0000-4000-8000-000000000307', 'Caja de galletas artesanales', 'Doce galletas artesanales de mantequilla en caja de regalo.', 18.00, TRUE, 25),
    ('00000000-0000-4000-8000-000000000308', 'Caja de galletas de chocolate', 'Galletas de chocolate con trozos de chocolate en caja decorada.', 16.00, TRUE, 30),
    ('00000000-0000-4000-8000-000000000309', 'Caja de chocolates surtidos', 'Surtido de chocolates finos en caja de 16 piezas.', 20.00, TRUE, 18),
    ('00000000-0000-4000-8000-000000000310', 'Arreglo de globos', 'Set de 15 globos de látex con cinta y base para regalo.', 22.00, TRUE, NULL),
    ('00000000-0000-4000-8000-000000000311', 'Arreglo de globos con peluche', 'Globos de látex con peluche pequeño para cumpleaños.', 30.00, TRUE, 8),
    ('00000000-0000-4000-8000-000000000312', 'Arreglo floral con globos', 'Combinación de flores de temporada y globos en una sola base.', 40.00, TRUE, 6)
ON CONFLICT (id) DO NOTHING;

INSERT INTO stock_movements (product_id, movement_type, delta, resulting_stock, reason)
SELECT id, 'RESTOCK', stock_quantity, stock_quantity, ''
FROM products
WHERE id BETWEEN '00000000-0000-4000-8000-000000000301' AND '00000000-0000-4000-8000-000000000312'
  AND stock_quantity IS NOT NULL;
