BEGIN;

ALTER TABLE public.checkout_order
    DROP COLUMN IF EXISTS discount_amount,
    DROP COLUMN IF EXISTS discount_code;

COMMIT;
