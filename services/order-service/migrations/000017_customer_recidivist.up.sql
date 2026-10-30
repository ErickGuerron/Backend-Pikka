-- Reincidence mark (ADR 0006, section 1). Set once, when the third incident of a window is reached,
-- and never overwritten: the first date stays. NULL means the customer is not a recidivist.
ALTER TABLE customers ADD COLUMN recidivist_at TIMESTAMPTZ;
