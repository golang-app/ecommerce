BEGIN;

ALTER TABLE auth_customer DROP COLUMN IF EXISTS must_change_password;

COMMIT;
