BEGIN;

CREATE TABLE auth_password_reset (
    token_hash text PRIMARY KEY,    -- sha256(token); we don't store the raw token
    customer_id text NOT NULL,      -- the customer's email
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz
);

CREATE INDEX auth_password_reset_customer_idx ON auth_password_reset(customer_id);

COMMIT;
