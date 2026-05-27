BEGIN;

ALTER TABLE productcatalog_product DROP COLUMN IF EXISTS attribute_set_id;

COMMIT;
