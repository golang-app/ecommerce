BEGIN;

CREATE TABLE public.cart_cart (
	user_id varchar NOT NULL,
	CONSTRAINT cart_cart_pk PRIMARY KEY (user_id)
);

CREATE TABLE public.cart_cart_item (
	id varchar NOT NULL,
	cart_id varchar NOT NULL,
	product_id varchar NOT NULL,
	product_name varchar NOT NULL,
	qty float8 NOT NULL,
	price int NOT NULL,
    currency varchar NOT NULL,
	CONSTRAINT cart_cart_item_pk PRIMARY KEY (id),
	CONSTRAINT cart_cart_item_fk FOREIGN KEY (cart_id) REFERENCES public.cart_cart(user_id)
);

COMMIT;
