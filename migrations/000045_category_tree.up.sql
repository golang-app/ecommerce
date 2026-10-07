BEGIN;

ALTER TABLE productcatalog_category
    ADD COLUMN parent_id text REFERENCES productcatalog_category(id) ON DELETE RESTRICT;

CREATE INDEX idx_productcatalog_category_parent_id ON productcatalog_category(parent_id);

COMMIT;
