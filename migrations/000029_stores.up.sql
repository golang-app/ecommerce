BEGIN;

-- store is the catalogue of storefront facades. The active store for any
-- given request is decided by matching the request Host header against
-- this table's `host` column; the store's `currency` then drives the
-- display layer. Exactly one row carries is_default=true — the partial
-- unique index below is the safety net, and the application's Create /
-- Update flow clears the flag on every other row when a caller marks
-- one default. The id is a stable, human-meaningful identifier supplied
-- by the operator (slug-like) so seeds/migrations can reference it.
CREATE TABLE public.store (
    id text PRIMARY KEY,
    slug text NOT NULL UNIQUE,
    name text NOT NULL,
    currency text NOT NULL,
    host text NOT NULL,
    is_default boolean NOT NULL DEFAULT false,
    position int NOT NULL DEFAULT 0
);

-- Exactly one default store across the table. The partial index makes
-- the constraint cheap (only one row participates) and lets the
-- application's "clear-other-defaults inside a tx" pattern succeed on
-- the happy path while still rejecting concurrent writers.
CREATE UNIQUE INDEX store_one_default_idx ON public.store ((is_default)) WHERE is_default;

-- store_host_idx supports the per-request "store by Host header" lookup.
-- The set is small (one row per facade) but the lookup runs on every
-- request, so the index keeps the cost trivial.
CREATE INDEX store_host_idx ON public.store(host);

COMMIT;
