BEGIN;

-- Shipping address captured at checkout, stored per-order. Nullable so the
-- legacy anonymous orders already in the table remain valid; new orders
-- always populate these (enforced in the domain).
ALTER TABLE public.checkout_order
    ADD COLUMN ship_name    varchar NULL,
    ADD COLUMN ship_street1 varchar NULL,
    ADD COLUMN ship_street2 varchar NULL,
    ADD COLUMN ship_city    varchar NULL,
    ADD COLUMN ship_zip     varchar NULL,
    ADD COLUMN ship_country varchar NULL;

COMMIT;
