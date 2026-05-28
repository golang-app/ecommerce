BEGIN;

-- wishlist_item holds the per-customer set of saved product variants. The
-- key is (customer_id, variant_id) — a variant is the purchasable unit so
-- different colours/sizes of the same product are independently saveable.
-- The ON DELETE CASCADE on the variant FK keeps the table tidy: when an
-- admin removes a variant from the catalogue, every wishlist entry that
-- referenced it disappears with it.
CREATE TABLE public.wishlist_item (
    customer_id text NOT NULL,
    variant_id text NOT NULL REFERENCES public.productcatalog_variant(id) ON DELETE CASCADE,
    added_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (customer_id, variant_id)
);

CREATE INDEX wishlist_item_customer_idx
    ON public.wishlist_item(customer_id, added_at DESC);

COMMIT;
