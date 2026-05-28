BEGIN;

DROP INDEX IF EXISTS productcatalog_stock_movement_order_idx;
DROP INDEX IF EXISTS productcatalog_stock_movement_variant_idx;
DROP TABLE IF EXISTS public.productcatalog_stock_movement;

COMMIT;
