BEGIN;

-- Option types a product offers (e.g. Color, Size). values is a JSON array
-- of the allowed values in display order.
CREATE TABLE public.productcatalog_option_type (
    id         varchar NOT NULL,
    product_id varchar NOT NULL,
    name       varchar NOT NULL,
    position   integer NOT NULL DEFAULT 0,
    values     jsonb   NOT NULL DEFAULT '[]'::jsonb,
    CONSTRAINT productcatalog_option_type_pk PRIMARY KEY (id),
    CONSTRAINT productcatalog_option_type_fk FOREIGN KEY (product_id)
        REFERENCES public.productcatalog_product(id) ON DELETE CASCADE
);
CREATE INDEX productcatalog_option_type_product_idx ON public.productcatalog_option_type(product_id);

-- Purchasable variants. options is a JSON object mapping option-type name to
-- the chosen value, e.g. {"Color":"Red","Size":"L"}. An empty object is a
-- default (single) variant.
CREATE TABLE public.productcatalog_variant (
    id             varchar NOT NULL,
    product_id     varchar NOT NULL,
    sku            varchar NOT NULL DEFAULT '',
    price_amount   bigint  NOT NULL,
    price_currency varchar NOT NULL,
    position       integer NOT NULL DEFAULT 0,
    options        jsonb   NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT productcatalog_variant_pk PRIMARY KEY (id),
    CONSTRAINT productcatalog_variant_fk FOREIGN KEY (product_id)
        REFERENCES public.productcatalog_product(id) ON DELETE CASCADE
);
CREATE INDEX productcatalog_variant_product_idx ON public.productcatalog_variant(product_id);

-- Give every existing product a default variant carrying its current price,
-- so products stay purchasable after price moves to the variant level.
INSERT INTO public.productcatalog_variant (id, product_id, sku, price_amount, price_currency, position, options)
SELECT 'var-' || id, id, id, price_amount, price_currency, 0, '{}'::jsonb
FROM public.productcatalog_product;

COMMIT;
