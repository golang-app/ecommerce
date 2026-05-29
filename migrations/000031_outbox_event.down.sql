BEGIN;

DROP INDEX IF EXISTS public.outbox_event_unsent_idx;
DROP TABLE IF EXISTS public.outbox_event;

COMMIT;
