BEGIN;

CREATE TABLE payments_provider_config (
    id text PRIMARY KEY,
    enabled boolean NOT NULL DEFAULT true,
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO payments_provider_config (id, enabled, config) VALUES
('stripe', true, '{"fail_card_ending_in":"0000","publishable_key":"","secret_key":"","webhook_secret":"whsec_dev_only_do_not_use_in_production"}'::jsonb),
('fake', true, '{"name":"Fake Payment Simulator","description":"Interactive payment simulator for testing payment outcomes."}'::jsonb);

ALTER TABLE payments_charge
    ADD COLUMN provider text NOT NULL DEFAULT 'stripe',
    ADD COLUMN order_id text NOT NULL DEFAULT '';

CREATE INDEX payments_charge_order_id_idx ON payments_charge(order_id) WHERE order_id <> '';

COMMIT;
