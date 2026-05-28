BEGIN;

CREATE TABLE productcatalog_attribute_set (
    id text PRIMARY KEY,
    name text NOT NULL,
    position int NOT NULL DEFAULT 0
);

CREATE TABLE productcatalog_attribute_set_item (
    set_id text NOT NULL REFERENCES productcatalog_attribute_set(id) ON DELETE CASCADE,
    attribute_type_id text NOT NULL REFERENCES productcatalog_attribute_type(id) ON DELETE CASCADE,
    position int NOT NULL DEFAULT 0,
    PRIMARY KEY (set_id, attribute_type_id)
);

COMMIT;
