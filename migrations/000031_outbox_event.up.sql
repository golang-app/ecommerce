BEGIN;

-- outbox_event is the durability backbone of the Transactional Outbox
-- pattern. Producers INSERT a row into this table inside the SAME
-- transaction that mutates their own state (here: appending checkout
-- events and projecting them), so the integration message and the
-- business fact share a single atomic commit. A separate dispatcher
-- polls unsent rows and publishes them onto the in-process bus,
-- giving at-least-once delivery across crashes between commit and
-- publish.
CREATE TABLE outbox_event (
    id bigserial PRIMARY KEY,
    kind text NOT NULL,
    payload bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    sent_at timestamptz
);

-- Partial index for the dispatcher's hot path: oldest unsent first.
-- WHERE sent_at IS NULL keeps the index tiny — once a row is marked
-- sent it falls out of the index entirely.
CREATE INDEX outbox_event_unsent_idx
    ON outbox_event(created_at)
    WHERE sent_at IS NULL;

COMMIT;
