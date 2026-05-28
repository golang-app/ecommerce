BEGIN;

-- Fulfillment metadata stored alongside the status. Carrier/tracking are
-- optional (empty string when not supplied). Existing rows default to ''.
ALTER TABLE public.checkout_order
    ADD COLUMN carrier text NOT NULL DEFAULT '',
    ADD COLUMN tracking_code text NOT NULL DEFAULT '';

COMMIT;
