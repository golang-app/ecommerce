BEGIN;

-- payments_charge is the payments bounded context's own record of every
-- charge attempted against the external provider. It is the durable
-- backing store the payments anti-corruption layer projects onto: the
-- domain.Charge value object reads/writes through this table only.
--
-- Why a dedicated table (rather than a column on `order`):
--   - the payments context owns its own state machine
--     (pending -> succeeded/failed) and must be reconcilable against the
--     provider independently of an order;
--   - the same Charge may be referenced by an async webhook arriving
--     long after the order aggregate has moved on (delayed captures,
--     disputes, refunds in a real Stripe wire-up);
--   - keeping payments data out of `order` preserves the
--     bounded-context boundary — checkout calls payments through a
--     port, not by JOINing on a column.
--
-- idempotency_key is UNIQUE and not the primary key: the same
-- (logical) charge may be retried by the caller with the same key, and
-- the service uses FindByIdempotencyKey to short-circuit a duplicate
-- before ever touching the provider. The PK stays on `id` (our domain
-- charge id) so future foreign keys point at a stable handle.
--
-- provider_ref is the external provider's identifier
-- (fakestripe PaymentIntent.ID in the demo; pi_xxx in real Stripe). It
-- is empty until the provider call returns; the webhook lookup uses
-- this column to find the matching Charge.
CREATE TABLE payments_charge (
    id text PRIMARY KEY,
    idempotency_key text UNIQUE,
    amount bigint NOT NULL,
    currency text NOT NULL,
    status text NOT NULL CHECK (status IN ('pending','succeeded','failed')),
    provider_ref text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- provider_ref is the column the webhook handler scans by when it
-- resolves the external event to our domain Charge. A partial index
-- on the non-empty rows keeps that lookup cheap regardless of how many
-- pending (not-yet-confirmed) charges have accumulated.
CREATE INDEX payments_charge_provider_ref_idx
    ON payments_charge(provider_ref)
    WHERE provider_ref <> '';

COMMIT;
