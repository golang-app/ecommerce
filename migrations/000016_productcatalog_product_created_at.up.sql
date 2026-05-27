BEGIN;

ALTER TABLE productcatalog_product ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();

COMMIT;
