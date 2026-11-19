-- Notices about the surcharge (ADR 0006, section 5): "se aplico un recargo" and "tu cuenta quedo bloqueada".
--
-- The block notice belongs to the account, not to an order, so order_id may be NULL for that kind only. The kind check
-- is dropped by looking its name up, not by assuming it: it was created inline and unnamed in migration 000005.
DO $$
DECLARE
    old_check text;
BEGIN
    SELECT conname INTO old_check
      FROM pg_constraint
     WHERE conrelid = 'customer_notifications'::regclass
       AND contype = 'c'
       AND pg_get_constraintdef(oid) LIKE '%ORDER_IN_REVIEW%';
    IF old_check IS NOT NULL THEN
        EXECUTE format('ALTER TABLE customer_notifications DROP CONSTRAINT %I', old_check);
    END IF;
END $$;

ALTER TABLE customer_notifications ALTER COLUMN order_id DROP NOT NULL;

ALTER TABLE customer_notifications ADD CONSTRAINT customer_notifications_kind_valid CHECK (kind IN (
    'ORDER_IN_REVIEW', 'ORDER_CONFIRMED', 'ORDER_CORRECTED', 'ORDER_REJECTED',
    'SURCHARGE_APPLIED', 'ACCOUNT_BLOCKED_BY_BALANCE'));

-- Every kind except the account block still names its order.
ALTER TABLE customer_notifications ADD CONSTRAINT customer_notifications_order_required CHECK (
    order_id IS NOT NULL OR kind = 'ACCOUNT_BLOCKED_BY_BALANCE');
