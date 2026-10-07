BEGIN;

DROP INDEX IF EXISTS idx_productcatalog_category_parent_id;

ALTER TABLE productcatalog_category
    DROP COLUMN IF EXISTS parent_id;

COMMIT;
