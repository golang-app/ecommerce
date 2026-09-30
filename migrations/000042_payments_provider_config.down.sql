BEGIN;

DROP INDEX IF EXISTS payments_charge_order_id_idx;

ALTER TABLE payments_charge
    DROP COLUMN IF EXISTS order_id,
    DROP COLUMN IF EXISTS provider;

DROP TABLE IF EXISTS payments_provider_config;

COMMIT;
