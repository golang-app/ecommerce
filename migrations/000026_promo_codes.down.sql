BEGIN;

DROP INDEX IF EXISTS public.promo_redemption_customer_idx;
DROP TABLE IF EXISTS public.promo_redemption;
DROP TABLE IF EXISTS public.promo_code;
DROP TYPE IF EXISTS promo_kind;

COMMIT;
