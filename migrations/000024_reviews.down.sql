BEGIN;

DROP INDEX IF EXISTS public.reviews_review_product_idx;
DROP INDEX IF EXISTS public.reviews_review_one_per_customer_per_product;
DROP TABLE IF EXISTS public.reviews_review;

COMMIT;
