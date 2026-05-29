BEGIN;

ALTER TABLE checkout_events
    DROP COLUMN payload_version;

COMMIT;
