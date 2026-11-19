-- Operator review of the automatic validation (ADR 0007, section 3) also covers
-- addresses that cannot be located on the map.
ALTER TABLE orders DROP CONSTRAINT orders_review_reasons_valid;
ALTER TABLE orders ADD CONSTRAINT orders_review_reasons_valid CHECK (
    review_reasons <@ ARRAY['ZONE_NOT_COVERED', 'INVALID_WINDOW', 'CAPACITY_EXCEEDED', 'CUSTOMER_BLOCKED', 'ADDRESS_NOT_LOCATED']::TEXT[]
    AND array_position(review_reasons, NULL) IS NULL
    AND ((status = 'PENDING_REVIEW' AND cardinality(review_reasons) > 0)
         OR (status <> 'PENDING_REVIEW' AND cardinality(review_reasons) = 0))
);

-- Notices for the customer about the review of their orders (ADR 0007, section 3).
-- The customer sees them in the app; push delivery is outside the MVP.
CREATE TABLE customer_notifications (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES customers (id) ON DELETE RESTRICT,
    order_id    UUID NOT NULL REFERENCES orders (id) ON DELETE RESTRICT,
    kind        TEXT NOT NULL CHECK (kind IN ('ORDER_IN_REVIEW', 'ORDER_CONFIRMED', 'ORDER_CORRECTED', 'ORDER_REJECTED')),
    detail      TEXT NOT NULL DEFAULT '' CHECK (char_length(detail) <= 300),
    read_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX customer_notifications_customer_idx ON customer_notifications (customer_id, created_at DESC, id);
