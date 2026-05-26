BEGIN;

-- Chosen shipping method snapshot. Code/label nullable for the legacy
-- orders already in the table; ship_cost defaults to 0 so their totals
-- (items-only) stay correct.
ALTER TABLE public.checkout_order
    ADD COLUMN ship_method_code  varchar NULL,
    ADD COLUMN ship_method_label varchar NULL,
    ADD COLUMN ship_cost         bigint NOT NULL DEFAULT 0;

COMMIT;
