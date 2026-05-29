BEGIN;

-- analytics_daily_sales is a second projection over the checkout event log:
-- a per-day, per-currency revenue counter incremented once for every
-- PaymentSucceeded event. The row is keyed on (day, currency) so multi-store
-- multi-currency revenue can be displayed separately. The table is fully
-- derived state — the rebuild-readmodels CLI truncates and re-projects it
-- from scratch in the same way as the checkout_order / _item read model.
CREATE TABLE analytics_daily_sales (
    day date NOT NULL,
    currency text NOT NULL,
    orders_count int NOT NULL DEFAULT 0,
    revenue_minor bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (day, currency)
);

COMMIT;
