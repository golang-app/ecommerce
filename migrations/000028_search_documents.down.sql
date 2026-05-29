BEGIN;

DROP INDEX IF EXISTS public.search_document_kind_idx;
DROP INDEX IF EXISTS public.search_document_ts_idx;
DROP TABLE IF EXISTS public.search_document;
DROP FUNCTION IF EXISTS public.search_document_refresh_ts();

COMMIT;
