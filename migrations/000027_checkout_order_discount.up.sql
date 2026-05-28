BEGIN;

-- Promo-code fields captured at place time. Both default to "no discount" so
-- historical orders (placed before promo codes existed) remain
-- arithmetic-identical and the projection's INSERT/UPSERT can keep treating
-- "no promo" as omitting the columns.
ALTER TABLE public.checkout_order
    ADD COLUMN discount_code text NOT NULL DEFAULT '',
    ADD COLUMN discount_amount bigint NOT NULL DEFAULT 0;

COMMIT;
