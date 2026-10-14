DROP INDEX IF EXISTS customer_notifications_customer_idx;
DROP TABLE IF EXISTS customer_notifications;

-- Fails if any order still carries ADDRESS_NOT_LOCATED: resolve those orders before rolling back.
ALTER TABLE orders DROP CONSTRAINT orders_review_reasons_valid;
ALTER TABLE orders ADD CONSTRAINT orders_review_reasons_valid CHECK (
    review_reasons <@ ARRAY['ZONE_NOT_COVERED', 'INVALID_WINDOW', 'CAPACITY_EXCEEDED', 'CUSTOMER_BLOCKED']::TEXT[]
    AND array_position(review_reasons, NULL) IS NULL
    AND ((status = 'PENDING_REVIEW' AND cardinality(review_reasons) > 0)
         OR (status <> 'PENDING_REVIEW' AND cardinality(review_reasons) = 0))
);
