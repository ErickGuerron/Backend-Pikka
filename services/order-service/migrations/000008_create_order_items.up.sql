-- Líneas de producto de un pedido (ADR 0009, sección 3). El precio y el nombre se copian al pedido.
CREATE TABLE order_items (
    order_id   UUID NOT NULL REFERENCES orders (id),
    product_id UUID NOT NULL REFERENCES products (id),
    name       VARCHAR(120) NOT NULL,
    unit_price NUMERIC(12, 2) NOT NULL,
    quantity   INTEGER NOT NULL,
    PRIMARY KEY (order_id, product_id),
    CONSTRAINT order_items_quantity_valid CHECK (quantity BETWEEN 1 AND 50),
    CONSTRAINT order_items_price_positive CHECK (unit_price > 0)
);
