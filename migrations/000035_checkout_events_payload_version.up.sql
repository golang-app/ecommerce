BEGIN;

-- payload_version makes the checkout event log version-aware so the codec
-- can pick the right DTO/upcaster for each row. Existing rows were written
-- under the v1 schema, so DEFAULT 1 captures their history correctly: an
-- upcaster will translate them into the latest in-memory shape at load time
-- (see backend/checkout/adapter/events_codec.go).
ALTER TABLE checkout_events
    ADD COLUMN payload_version int NOT NULL DEFAULT 1;

COMMIT;
