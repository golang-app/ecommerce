BEGIN;

CREATE TABLE productcatalog_attribute_type (
    id text PRIMARY KEY,
    name text NOT NULL,
    unit text NOT NULL DEFAULT '',
    kind text NOT NULL CHECK (kind IN ('numeric','enum')),
    filterable boolean NOT NULL DEFAULT false,
    position int NOT NULL DEFAULT 0
);

CREATE TABLE productcatalog_product_attribute (
    product_id text NOT NULL REFERENCES productcatalog_product(id) ON DELETE CASCADE,
    attribute_type_id text NOT NULL REFERENCES productcatalog_attribute_type(id) ON DELETE CASCADE,
    num_value numeric,
    text_value text,
    PRIMARY KEY (product_id, attribute_type_id)
);

CREATE TABLE productcatalog_category (
    id text PRIMARY KEY,
    name text NOT NULL,
    slug text NOT NULL UNIQUE,
    position int NOT NULL DEFAULT 0
);

CREATE TABLE productcatalog_product_category (
    product_id text NOT NULL REFERENCES productcatalog_product(id) ON DELETE CASCADE,
    category_id text NOT NULL REFERENCES productcatalog_category(id) ON DELETE CASCADE,
    PRIMARY KEY (product_id, category_id)
);

COMMIT;
