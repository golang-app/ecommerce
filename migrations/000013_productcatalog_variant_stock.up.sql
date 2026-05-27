BEGIN;

ALTER TABLE public.productcatalog_variant
    ADD COLUMN stock integer NOT NULL DEFAULT 0;

-- Existing variants start well-stocked so nothing is unexpectedly
-- unavailable after the column is added.
UPDATE public.productcatalog_variant SET stock = 100;

COMMIT;
