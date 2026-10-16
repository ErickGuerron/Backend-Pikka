-- Contact data asked at signup and delivery details asked per order. Rows created before this
-- migration (and the seed customer) keep an empty value, so the columns are NOT NULL DEFAULT ''.
ALTER TABLE customers
    ADD COLUMN phone TEXT NOT NULL DEFAULT '',
    ADD COLUMN city  TEXT NOT NULL DEFAULT '';

ALTER TABLE orders
    ADD COLUMN address_reference TEXT NOT NULL DEFAULT '',
    ADD COLUMN postal_code       TEXT NOT NULL DEFAULT '',
    ADD COLUMN contact_phone     TEXT NOT NULL DEFAULT '';
