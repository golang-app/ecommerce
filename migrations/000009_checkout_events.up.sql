BEGIN;

-- Append-only event store for the checkout/ordering context. The order
-- read model (checkout_order / checkout_order_item) is a projection built
-- from these events. PK on (aggregate_id, sequence) gives per-aggregate
-- ordering and optimistic-concurrency protection on append.
CREATE TABLE public.checkout_events (
    aggregate_id varchar      NOT NULL,
    sequence     integer      NOT NULL,
    event_type   varchar      NOT NULL,
    payload      jsonb        NOT NULL,
    occurred_at  timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT checkout_events_pk PRIMARY KEY (aggregate_id, sequence)
);

COMMIT;
