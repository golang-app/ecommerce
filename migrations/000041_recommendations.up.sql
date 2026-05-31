BEGIN;

-- recommendation_link is the materialised top-N read model for the
-- "you might also like" section on the storefront. Each row links a
-- seed product to one recommended product, with the score the
-- refresher last computed and the position (0..N-1) inside the seed's
-- top-N. The refresher's UpsertTopN replaces the whole list for a
-- seed under a transaction so a reader either sees the previous list
-- in full or the next one in full — never a half-built mix.
CREATE TABLE public.recommendation_link (
    product_id     text NOT NULL,
    recommended_id text NOT NULL,
    score          real NOT NULL,
    position       int  NOT NULL,
    computed_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (product_id, recommended_id)
);

-- Lookup index for the storefront read path: TopN(productID, limit)
-- selects the rows for one seed ordered by position. Keeps the hot
-- query plan a single index range scan.
CREATE INDEX recommendation_link_lookup_idx
    ON public.recommendation_link(product_id, position);

-- recommendation_weights is the singleton row holding the five
-- admin-tunable scorer weights. Forcing id = 1 with a CHECK constraint
-- guarantees at-most-one row; the seed INSERT below makes the row
-- present from migration time so the admin Get handler can rely on it
-- existing.
CREATE TABLE public.recommendation_weights (
    id                 int PRIMARY KEY CHECK (id = 1),
    weight_copurchase  real NOT NULL DEFAULT 0.40,
    weight_text        real NOT NULL DEFAULT 0.20,
    weight_category    real NOT NULL DEFAULT 0.20,
    weight_attributes  real NOT NULL DEFAULT 0.10,
    weight_price       real NOT NULL DEFAULT 0.10,
    updated_at         timestamptz NOT NULL DEFAULT now()
);

INSERT INTO public.recommendation_weights(id) VALUES (1)
    ON CONFLICT (id) DO NOTHING;

COMMIT;
