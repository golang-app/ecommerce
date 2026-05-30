BEGIN;

-- idempotency_key is the HTTP-boundary record of (client-supplied key,
-- recorded response) tuples that lets the application return the SAME
-- response body+status for a retried POST/PUT/PATCH/DELETE without
-- re-executing the handler. Pairs with the Outbox + Inbox: those give
-- exactly-once semantics for server-to-server event propagation; this
-- table closes the equivalent loop for client-driven actions where the
-- client may legitimately retry on a flaky network.
--
-- key is opaque text supplied by the client via the Idempotency-Key
-- request header; the column is the primary key so a retry collides
-- with the original row and ON CONFLICT DO NOTHING preserves the first
-- caller's recorded response.
--
-- response_body is bytea (not text) because handler output is allowed
-- to be HTML, JSON, or any other byte sequence; response_headers is
-- jsonb so the middleware can faithfully replay Content-Type and any
-- HX-* headers a downstream HTMX swap depends on.
--
-- expires_at defaults to now()+24h: the table is purely a retry-safety
-- cache, not a long-lived audit log, so a generous-but-bounded TTL
-- keeps the surface small without sacrificing the operationally
-- interesting "the same client retried within a reasonable window"
-- case.
CREATE TABLE idempotency_key (
    key text PRIMARY KEY,
    method text NOT NULL,
    path text NOT NULL,
    status_code int NOT NULL,
    response_body bytea NOT NULL,
    response_headers jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT (now() + interval '24 hours')
);

-- Plain B-tree on expires_at supports the routine maintenance sweep
-- `DELETE FROM idempotency_key WHERE expires_at < now()`. We can't use
-- a partial index with a now()-based predicate (Postgres rejects it
-- because now() is STABLE, not IMMUTABLE). The hot path uses the PK
-- anyway; the expires_at index is only for the sweeper.
CREATE INDEX idempotency_key_expires_idx ON idempotency_key(expires_at);

COMMIT;
