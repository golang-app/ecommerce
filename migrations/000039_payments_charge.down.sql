BEGIN;

DROP INDEX IF EXISTS public.payments_charge_provider_ref_idx;
DROP TABLE IF EXISTS public.payments_charge;

COMMIT;
