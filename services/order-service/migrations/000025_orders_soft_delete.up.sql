-- Borrado logico de pedidos: el pedido nunca se borra fisicamente; deleted_at marca la baja.
-- Las lecturas de la aplicacion excluyen los pedidos con deleted_at (ver order_repository.go).
ALTER TABLE orders ADD COLUMN deleted_at TIMESTAMPTZ NULL;

CREATE INDEX idx_orders_active ON orders (created_at) WHERE deleted_at IS NULL;