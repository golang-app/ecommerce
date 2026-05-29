BEGIN;

-- reviews_review.status moves the context from "auto-publish" to a small
-- moderation queue: submissions land as 'pending' and an admin flips them
-- to 'approved' or 'rejected'. The DEFAULT here only ever applies to
-- historical rows (which were auto-published, so 'approved' is the correct
-- back-compat); the application layer always passes the status explicitly
-- on Insert, so new submissions are 'pending' regardless of the default.
ALTER TABLE public.reviews_review
    ADD COLUMN status text NOT NULL DEFAULT 'approved'
    CHECK (status IN ('pending','approved','rejected'));

-- Partial index that powers the default admin moderation queue
-- (status=pending, newest first). Filtering on deleted_at IS NULL keeps
-- the index lean — soft-deleted rows don't belong to any tab.
CREATE INDEX reviews_review_pending_idx
    ON public.reviews_review(created_at DESC)
    WHERE status = 'pending' AND deleted_at IS NULL;

COMMIT;
