BEGIN;

DROP INDEX IF EXISTS auth_password_reset_customer_idx;
DROP TABLE IF EXISTS auth_password_reset;

COMMIT;
