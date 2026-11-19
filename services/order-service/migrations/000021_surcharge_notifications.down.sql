-- The new kinds cannot exist under the old constraints, so their notices are removed first.
DELETE FROM customer_notifications WHERE kind IN ('SURCHARGE_APPLIED', 'ACCOUNT_BLOCKED_BY_BALANCE');

ALTER TABLE customer_notifications DROP CONSTRAINT IF EXISTS customer_notifications_order_required;
ALTER TABLE customer_notifications DROP CONSTRAINT IF EXISTS customer_notifications_kind_valid;
ALTER TABLE customer_notifications ADD CONSTRAINT customer_notifications_kind_check CHECK (
    kind IN ('ORDER_IN_REVIEW', 'ORDER_CONFIRMED', 'ORDER_CORRECTED', 'ORDER_REJECTED'));

ALTER TABLE customer_notifications ALTER COLUMN order_id SET NOT NULL;
