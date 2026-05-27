BEGIN;

ALTER TABLE productcatalog_product DROP COLUMN created_at;

COMMIT;
