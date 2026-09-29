BEGIN;

CREATE TYPE product_image_position AS ENUM ('PRIMARY', 'GALLERY');

CREATE TABLE public.productcatalog_product_image (
    id text NOT NULL,
    product_id text NOT NULL,
    url text NOT NULL,
    position product_image_position NOT NULL DEFAULT 'GALLERY',
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT productcatalog_product_image_pk PRIMARY KEY (id),
    CONSTRAINT productcatalog_product_image_product_fk 
        FOREIGN KEY (product_id) REFERENCES public.productcatalog_product(id) ON DELETE CASCADE
);

CREATE INDEX idx_productcatalog_product_image_product_id 
    ON public.productcatalog_product_image(product_id);

COMMIT;
