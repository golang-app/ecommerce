BEGIN;

CREATE TABLE public.checkout_order (
    id varchar NOT NULL,
    user_id varchar NOT NULL,
    total_amount bigint NOT NULL,
    total_currency varchar NOT NULL,
    status varchar NOT NULL,
    placed_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT checkout_order_pk PRIMARY KEY (id)
);

CREATE INDEX checkout_order_user_id_idx ON public.checkout_order(user_id);

CREATE TABLE public.checkout_order_item (
    id varchar NOT NULL,
    order_id varchar NOT NULL,
    product_id varchar NOT NULL,
    product_name varchar NOT NULL,
    qty integer NOT NULL,
    price_amount bigint NOT NULL,
    price_currency varchar NOT NULL,
    CONSTRAINT checkout_order_item_pk PRIMARY KEY (id),
    CONSTRAINT checkout_order_item_fk FOREIGN KEY (order_id) REFERENCES public.checkout_order(id) ON DELETE CASCADE
);

CREATE INDEX checkout_order_item_order_id_idx ON public.checkout_order_item(order_id);

COMMIT;
