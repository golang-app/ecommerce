BEGIN;

-- Saved shipping addresses (address book) owned by a customer. The first
-- address a customer adds becomes their default; exactly one default per
-- customer is expected (enforced in the application layer).
CREATE TABLE public.shippinginfo_address (
    id          varchar     NOT NULL,
    customer_id varchar     NOT NULL,
    name        varchar     NOT NULL,
    street1     varchar     NOT NULL,
    street2     varchar     NOT NULL DEFAULT '',
    city        varchar     NOT NULL,
    zip         varchar     NOT NULL,
    country     varchar     NOT NULL,
    is_default  boolean     NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT shippinginfo_address_pk PRIMARY KEY (id)
);

CREATE INDEX shippinginfo_address_customer_idx ON public.shippinginfo_address(customer_id);

COMMIT;
