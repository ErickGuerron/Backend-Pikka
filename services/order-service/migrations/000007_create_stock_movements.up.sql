-- Historial inmutable de stock (ADR 0009, sección 2). No se edita ni se borra desde la aplicación.
CREATE TABLE stock_movements (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id      UUID NOT NULL REFERENCES products (id),
    movement_type   VARCHAR(20) NOT NULL,
    delta           BIGINT NOT NULL,
    resulting_stock BIGINT NOT NULL,
    reason          TEXT NOT NULL DEFAULT '',
    order_id        UUID REFERENCES orders (id),
    actor_user_id   UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT stock_movements_type_valid CHECK (movement_type IN ('RESTOCK', 'ADJUSTMENT', 'SALE', 'RETURN')),
    CONSTRAINT stock_movements_result_non_negative CHECK (resulting_stock >= 0),
    -- El signo y el motivo deben corresponder al tipo.
    CONSTRAINT stock_movements_sign_matches_type CHECK (
        (movement_type IN ('RESTOCK', 'RETURN') AND delta > 0)
        OR (movement_type = 'SALE' AND delta < 0)
        OR (movement_type = 'ADJUSTMENT' AND delta <> 0 AND btrim(reason) <> '')
    )
);

CREATE INDEX stock_movements_product_idx ON stock_movements (product_id, created_at);
