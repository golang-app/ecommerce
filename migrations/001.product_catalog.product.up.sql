CREATE TABLE public.productcatalog_product (
	id text NOT NULL,
	"name" varchar NOT NULL,
	description varchar NOT NULL,
	thumbnail varchar NOT NULL,
	price_amount int NOT NULL,
	price_currency varchar NOT NULL,
	CONSTRAINT productcatalog_product_pk PRIMARY KEY (id)
);
