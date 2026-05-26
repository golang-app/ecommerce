BEGIN;

-- Chosen payment method snapshot. Nullable for the legacy orders already
-- in the table.
ALTER TABLE public.checkout_order
    ADD COLUMN payment_method_code  varchar NULL,
    ADD COLUMN payment_method_label varchar NULL;

COMMIT;
