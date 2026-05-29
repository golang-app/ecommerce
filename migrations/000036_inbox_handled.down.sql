BEGIN;

DROP INDEX IF EXISTS public.inbox_handled_subscriber_idx;
DROP TABLE IF EXISTS public.inbox_handled;

COMMIT;
