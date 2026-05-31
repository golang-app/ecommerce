BEGIN;

-- repricing is the state-stored projection driven by the repricing
-- bounded context (Process Manager pattern). Each row represents one
-- bulk "reprice every product in a category by N%" operation; the
-- saga walks the planned product ids and advances processed_items
-- under optimistic concurrency until the run completes (or fails).
--
-- The version column drives the OCC check: the adapter's UPDATE
-- matches on the expected previous version and a mismatch surfaces
-- as app.ErrOptimisticLock.
CREATE TABLE repricing (
    id              text        PRIMARY KEY,
    category_id     text        NOT NULL,
    percent_change  real        NOT NULL,
    status          text        NOT NULL CHECK (status IN ('scheduled','in_progress','completed','failed')),
    total_items     int         NOT NULL DEFAULT 0,
    processed_items int         NOT NULL DEFAULT 0,
    last_error      text        NOT NULL DEFAULT '',
    started_at      timestamptz NOT NULL DEFAULT now(),
    completed_at    timestamptz,
    version         int         NOT NULL DEFAULT 1
);

-- Partial unique index enforces "only one active reprice at a time".
-- The application service also checks via FindActive before issuing
-- Create; the DB-level guard is the belt-and-braces second check
-- against two concurrent admins clicking the button at the same
-- time.
CREATE UNIQUE INDEX repricing_one_active_idx
    ON repricing(status)
    WHERE status IN ('scheduled', 'in_progress');

COMMIT;
