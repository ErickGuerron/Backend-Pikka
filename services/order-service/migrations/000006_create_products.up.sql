-- Run with order_service_user and search_path=orders,public.
-- Catálogo de productos (ADR 0009). stock_quantity nulo significa que el producto no controla stock.
CREATE TABLE products (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name           VARCHAR(120) NOT NULL,
    description    VARCHAR(500) NOT NULL DEFAULT '',
    unit_price     NUMERIC(12, 2) NOT NULL,
    active         BOOLEAN NOT NULL DEFAULT TRUE,
    stock_quantity BIGINT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT products_name_valid CHECK (char_length(btrim(name)) BETWEEN 1 AND 120),
    CONSTRAINT products_price_positive CHECK (unit_price > 0),
    CONSTRAINT products_stock_non_negative CHECK (stock_quantity IS NULL OR stock_quantity >= 0)
);

CREATE INDEX products_active_name_idx ON products (name) WHERE active;
