ALTER TABLE orders
    DROP COLUMN IF EXISTS contact_phone,
    DROP COLUMN IF EXISTS postal_code,
    DROP COLUMN IF EXISTS address_reference;

ALTER TABLE customers
    DROP COLUMN IF EXISTS city,
    DROP COLUMN IF EXISTS phone;
