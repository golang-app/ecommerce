BEGIN;

ALTER TABLE public.checkout_order
    DROP COLUMN IF EXISTS tax_amount;

COMMIT;
