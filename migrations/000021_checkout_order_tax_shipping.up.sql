BEGIN;

-- Tax amount captured at place time, in minor units. Existing rows default
-- to 0 (no tax) so historical orders' totals remain unchanged. The
-- ship_cost column already exists (000007_checkout_order_shipping_method).
ALTER TABLE public.checkout_order
    ADD COLUMN tax_amount bigint NOT NULL DEFAULT 0;

COMMIT;
