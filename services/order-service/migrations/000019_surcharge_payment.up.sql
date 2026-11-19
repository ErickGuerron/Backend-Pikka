-- When a balance is paid (ADR 0006, section 5) every unpaid incident and recidivism charge is stamped.
-- NULL means still unpaid. The overdue-balance sweep reads recidivism_charges.paid_at instead of guessing
-- from the customer's balance, which new surcharges also raise.
ALTER TABLE surcharges ADD COLUMN paid_at TIMESTAMPTZ;
ALTER TABLE recidivism_charges ADD COLUMN paid_at TIMESTAMPTZ;
