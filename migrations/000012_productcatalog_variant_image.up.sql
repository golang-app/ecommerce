BEGIN;

ALTER TABLE public.productcatalog_variant
    ADD COLUMN image_url varchar NOT NULL DEFAULT '';

-- Existing variants (the default ones created for pre-variant products)
-- inherit their product's thumbnail so the product page still shows an image.
UPDATE public.productcatalog_variant v
SET image_url = p.thumbnail
FROM public.productcatalog_product p
WHERE v.product_id = p.id AND v.image_url = '';

COMMIT;
