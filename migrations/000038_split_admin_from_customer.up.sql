BEGIN;

-- 000038_split_admin_from_customer splits the conflated `auth_customer`
-- identity into two separate aggregates inside the auth bounded context:
--
--   - Customer (commercial actor): keeps `auth_customer`, minus the two
--     admin-only columns is_admin / must_change_password.
--   - Admin (operator): a brand-new `auth_admin` table with its own id,
--     password hash, role, and must_change_password gate.
--
-- Sessions stay on a single table (`auth_session`) but gain a
-- `principal_kind` discriminator so a customer session and an admin
-- session can coexist for the same browser without crossing the streams.
-- Cookies are kept separate in the layout layer (ecommerce /
-- ecommerce_admin), but storage-wise this is one table.
--
-- NOTE on orphan rows in other contexts:
-- `customer_id` columns in checkout / reviews / wishlist / etc. are
-- plain text and reference an email string. After this migration the
-- email of any previously-promoted admin is no longer present in
-- `auth_customer`, so any orders/reviews/wishlist items they created
-- during demo runs effectively reference a customer that does not
-- exist anymore. That is intentional — the columns are not declared
-- as foreign keys, the rows are functionally inert (an admin can no
-- longer log in as a customer to view them), and the demo seed
-- wipes those tables on re-run anyway.

-- 1. Create the new admin table. id is the natural key; we use the email
--    as id today so the migration is straight-forward, but the column is
--    declared as plain text so a later migration can swap in a synthetic
--    id without touching the schema.
CREATE TABLE auth_admin (
    id text PRIMARY KEY,
    email text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    role text NOT NULL DEFAULT 'admin',
    must_change_password boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- 2. Migrate any rows from auth_customer where is_admin=true into
--    auth_admin. id := username (email); ON CONFLICT DO NOTHING keeps
--    the migration safely re-runnable across dev resets.
INSERT INTO auth_admin (id, email, password_hash, role, must_change_password)
SELECT username, username, password_hash, 'admin', must_change_password
FROM auth_customer
WHERE is_admin = true
ON CONFLICT (id) DO NOTHING;

-- 3. Delete the promoted rows from auth_customer. The split is the
--    whole point of this migration — keeping a copy in both tables
--    would re-introduce the conflated identity we are removing.
DELETE FROM auth_customer WHERE is_admin = true;

-- 4. Drop the now-conflated columns from auth_customer.
ALTER TABLE auth_customer DROP COLUMN is_admin;
ALTER TABLE auth_customer DROP COLUMN must_change_password;

-- 5. Tag existing auth_session rows as customer-owned. New admin
--    sessions will be written with principal_kind='admin' by the
--    admin auth service; the customer service keeps writing
--    'customer'. The column carries a non-null default so the
--    add-column is online.
ALTER TABLE auth_session ADD COLUMN principal_kind text NOT NULL DEFAULT 'customer';

COMMIT;
