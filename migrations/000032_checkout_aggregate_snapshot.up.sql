BEGIN;

-- checkout_aggregate_snapshot caches the folded state of an order aggregate
-- at a known event version, so Load can skip replaying the entire event log
-- for long-lived aggregates. payload is a JSON-encoded DTO (see the
-- checkout/adapter snapshot codec) that mirrors the domain.Order's getters
-- one-to-one. The snapshot is best-effort: if it goes missing the adapter
-- still falls back to a full replay of checkout_events.
CREATE TABLE checkout_aggregate_snapshot (
    aggregate_id text PRIMARY KEY,
    version int NOT NULL,
    payload bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

COMMIT;
