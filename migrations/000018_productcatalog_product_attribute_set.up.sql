BEGIN;

ALTER TABLE productcatalog_product
    ADD COLUMN attribute_set_id text REFERENCES productcatalog_attribute_set(id) ON DELETE SET NULL;

COMMIT;
