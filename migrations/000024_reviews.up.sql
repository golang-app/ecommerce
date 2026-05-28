BEGIN;

-- reviews_review holds customer-submitted ratings and bodies for products.
-- Only verified buyers can create rows (enforced in the application layer
-- by joining checkout_order_item -> productcatalog_variant -> product_id).
-- One review per (customer, product) is enforced by the partial unique
-- index below, which ignores soft-deleted rows so a buyer whose review was
-- removed by an admin can submit a fresh one.
CREATE TABLE public.reviews_review (
    id text PRIMARY KEY,
    product_id text NOT NULL REFERENCES public.productcatalog_product(id) ON DELETE CASCADE,
    customer_id text NOT NULL,
    rating int NOT NULL CHECK (rating BETWEEN 1 AND 5),
    body text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE UNIQUE INDEX reviews_review_one_per_customer_per_product
    ON public.reviews_review(product_id, customer_id)
    WHERE deleted_at IS NULL;

CREATE INDEX reviews_review_product_idx
    ON public.reviews_review(product_id, created_at DESC)
    WHERE deleted_at IS NULL;

COMMIT;
