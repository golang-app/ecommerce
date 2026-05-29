BEGIN;

-- fulfillment is the state-stored projection driven by the
-- fulfillment bounded context (Process Manager pattern). Each row is
-- spawned by the OnOrderPaid subscriber (one per order, enforced by
-- the unique constraint on order_id) and advances through the
-- operational lifecycle scheduled -> labeled -> shipped -> delivered,
-- with a refund branch reachable from any active state. The version
-- column drives optimistic concurrency: the adapter's UPDATE matches
-- on the expected previous version and a mismatch surfaces as
-- app.ErrOptimisticLock.
--
-- The 'returned' status value is reserved for a future inbound-return
-- flow (parcel scanned back into the warehouse); accepting it in the
-- CHECK constraint today means that flow can land without a follow-up
-- migration.
CREATE TABLE fulfillment (
    id            text        PRIMARY KEY,
    order_id      text        NOT NULL UNIQUE,
    status        text        NOT NULL CHECK (status IN ('scheduled','labeled','shipped','delivered','returned','refunded')),
    carrier       text        NOT NULL DEFAULT '',
    tracking_code text        NOT NULL DEFAULT '',
    scheduled_at  timestamptz,
    shipped_at    timestamptz,
    delivered_at  timestamptz,
    refund_reason text        NOT NULL DEFAULT '',
    version       int         NOT NULL DEFAULT 1
);

CREATE INDEX fulfillment_status_idx ON fulfillment(status);

COMMIT;
