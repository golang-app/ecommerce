BEGIN;

ALTER TABLE auth_customer ADD COLUMN must_change_password boolean NOT NULL DEFAULT false;

COMMIT;
