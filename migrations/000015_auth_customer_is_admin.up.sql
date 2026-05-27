BEGIN;

ALTER TABLE auth_customer ADD COLUMN is_admin boolean NOT NULL DEFAULT false;

COMMIT;
