BEGIN;

DROP INDEX IF EXISTS public.wishlist_item_customer_idx;
DROP TABLE IF EXISTS public.wishlist_item;

COMMIT;
