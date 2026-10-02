CREATE TABLE IF NOT EXISTS rate_limits (
    action VARCHAR(64) NOT NULL,
    key VARCHAR(128) NOT NULL,
    tokens DOUBLE PRECISION NOT NULL,
    last_refill TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (action, key)
);
