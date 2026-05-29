BEGIN;

-- inbox_handled is the durability backbone of the Inbox pattern, the
-- consumer-side mirror of the Transactional Outbox table. The outbox
-- guarantees at-least-once delivery from the producer; the inbox
-- records per-subscriber which originating event ids have already
-- been processed so a redelivery is a cheap no-op instead of a
-- repeated side effect.
--
-- (subscriber, event_id) is the composite primary key: each
-- subscriber sees the same event id at most once, but two different
-- subscribers can independently handle the same originating event.
-- event_id is the outbox_event.id of the originating row so the key
-- is content-stable across redeliveries and process restarts.
CREATE TABLE inbox_handled (
    subscriber text NOT NULL,
    event_id bigint NOT NULL,
    handled_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (subscriber, event_id)
);

-- Operator-facing index: "show me the last N events handled by
-- subscriber X". Not needed by the hot path (the primary key already
-- covers the dedupe lookup) but cheap to maintain and invaluable for
-- post-incident inspection.
CREATE INDEX inbox_handled_subscriber_idx
    ON inbox_handled(subscriber, handled_at DESC);

COMMIT;
