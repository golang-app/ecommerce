BEGIN;

CREATE TABLE public.productcatalog_stock_movement (
    id bigserial PRIMARY KEY,
    variant_id text NOT NULL,
    delta int NOT NULL,
    reason text NOT NULL,
    ref_order_id text NOT NULL DEFAULT '',
    at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX productcatalog_stock_movement_variant_idx
    ON public.productcatalog_stock_movement(variant_id);
CREATE INDEX productcatalog_stock_movement_order_idx
    ON public.productcatalog_stock_movement(ref_order_id) WHERE ref_order_id <> '';

COMMIT;
