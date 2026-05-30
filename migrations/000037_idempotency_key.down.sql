BEGIN;

DROP INDEX IF EXISTS public.idempotency_key_expires_idx;
DROP TABLE IF EXISTS public.idempotency_key;

COMMIT;
