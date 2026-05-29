BEGIN;

DROP INDEX IF EXISTS public.reviews_review_pending_idx;
ALTER TABLE public.reviews_review DROP COLUMN IF EXISTS status;

COMMIT;
