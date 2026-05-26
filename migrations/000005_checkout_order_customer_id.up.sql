BEGIN;

ALTER TABLE public.checkout_order
    ADD COLUMN customer_id varchar NULL;

-- Partial index — most orders eventually get a customer_id once anonymous
-- checkout is restricted, but the nullable column accommodates the legacy
-- anonymous orders already in the table.
CREATE INDEX checkout_order_customer_id_idx
    ON public.checkout_order(customer_id)
    WHERE customer_id IS NOT NULL;

COMMIT;
