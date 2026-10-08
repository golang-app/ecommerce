CREATE TABLE IF NOT EXISTS widget (
    id VARCHAR(36) PRIMARY KEY,
    page VARCHAR(50) NOT NULL,
    zone VARCHAR(50) NOT NULL,
    type VARCHAR(50) NOT NULL,
    title VARCHAR(255) NOT NULL,
    config JSONB NOT NULL DEFAULT '{}',
    display_order INT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_widget_page_zone_active_order ON widget (page, zone, is_active, display_order ASC);

CREATE TABLE IF NOT EXISTS widget_slide (
    id VARCHAR(36) PRIMARY KEY,
    widget_id VARCHAR(36) NOT NULL REFERENCES widget(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL DEFAULT '',
    subtitle VARCHAR(255) NOT NULL DEFAULT '',
    link_url VARCHAR(500) NOT NULL DEFAULT '',
    link_text VARCHAR(100) NOT NULL DEFAULT '',
    promo_code VARCHAR(50) NOT NULL DEFAULT '',
    badge VARCHAR(50) NOT NULL DEFAULT '',
    image_url VARCHAR(500) NOT NULL DEFAULT '',
    display_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_widget_slide_widget_order ON widget_slide (widget_id, display_order ASC);

DROP TABLE IF EXISTS banner CASCADE;
