BEGIN;

ALTER TABLE public.checkout_order
    DROP COLUMN IF EXISTS carrier,
    DROP COLUMN IF EXISTS tracking_code;

COMMIT;
