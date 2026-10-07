CREATE TABLE IF NOT EXISTS banner (
    id VARCHAR(36) PRIMARY KEY,
    slot VARCHAR(50) NOT NULL,
    title VARCHAR(255) NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    link_url VARCHAR(500) NOT NULL DEFAULT '',
    link_text VARCHAR(100) NOT NULL DEFAULT '',
    promo_code VARCHAR(50) NOT NULL DEFAULT '',
    icon VARCHAR(50) NOT NULL DEFAULT '',
    image_url VARCHAR(500) NOT NULL DEFAULT '',
    display_order INT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_banner_slot_active_order ON banner (slot, is_active, display_order ASC);
