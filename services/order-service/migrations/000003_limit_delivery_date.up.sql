-- PostgreSQL accepts BC dates and years beyond four digits. The public contract
-- and the Go civil-date parser require YYYY-MM-DD with years 0001 through 9999.
ALTER TABLE orders ADD CONSTRAINT orders_delivery_date_contract_range
    CHECK (delivery_date BETWEEN DATE '0001-01-01' AND DATE '9999-12-31');
