BEGIN;

-- Reverse of 000038_split_admin_from_customer. Best-effort: the down
-- file's job is to make local development resets work, not to perform
-- a production-grade rollback. Two notes:
--   * We restore is_admin / must_change_password with their original
--     defaults (false) and copy back any auth_admin rows. Any
--     auth_admin rows whose id is not a valid email will land in
--     auth_customer with that same string as username; that's
--     consistent with the forward migration's id := email choice.
--   * The principal_kind column is dropped wholesale; we don't try
--     to filter sessions by it on the way down because the customer
--     service would happily resolve them anyway.

-- 1. Re-add the dropped columns.
ALTER TABLE auth_customer ADD COLUMN is_admin boolean NOT NULL DEFAULT false;
ALTER TABLE auth_customer ADD COLUMN must_change_password boolean NOT NULL DEFAULT false;

-- 2. Copy admin rows back into auth_customer. The forward migration
--    deleted these on the way up, so any admin we restore here was
--    in auth_customer originally.
INSERT INTO auth_customer (username, password_hash, is_admin, must_change_password)
SELECT email, password_hash, true, must_change_password
FROM auth_admin
ON CONFLICT (username) DO UPDATE SET
    is_admin = true,
    must_change_password = EXCLUDED.must_change_password;

-- 3. Drop the new admin table.
DROP TABLE auth_admin;

-- 4. Drop the session discriminator.
ALTER TABLE auth_session DROP COLUMN principal_kind;

COMMIT;
