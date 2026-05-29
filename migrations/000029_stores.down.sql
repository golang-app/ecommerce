BEGIN;

DROP INDEX IF EXISTS public.store_host_idx;
DROP INDEX IF EXISTS public.store_one_default_idx;
DROP TABLE IF EXISTS public.store;

COMMIT;
