CREATE TABLE IF NOT EXISTS shipping_method_config (
    code VARCHAR(64) PRIMARY KEY,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    label VARCHAR(255) NOT NULL,
    cost BIGINT NOT NULL DEFAULT 0,
    requires_address BOOLEAN NOT NULL DEFAULT TRUE,
    carrier VARCHAR(255) NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO shipping_method_config (code, enabled, label, cost, requires_address, carrier, updated_at)
VALUES
    ('flat', true, 'Flat rate', 500, true, 'Standard Post', NOW()),
    ('pickup', true, 'Personal pickup', 0, false, 'Store Pickup', NOW()),
    ('courier', true, 'Courier', 1500, true, 'Express Courier', NOW())
ON CONFLICT (code) DO NOTHING;
