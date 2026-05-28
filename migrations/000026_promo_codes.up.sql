BEGIN;

-- promo_code is the catalogue of discount codes admins can create. The PK is
-- the code itself (text) — codes are surfaced to customers verbatim and there
-- is no benefit to a synthetic id. value_minor carries the per-kind payload:
-- for 'percent' it is 1..100 (whole-percent), for 'fixed' it is the discount
-- amount in minor units (so e.g. 500 = $5.00), and for 'free_shipping' it is
-- ignored. used_count is the running redemption tally; max_uses=0 means
-- unlimited and per_customer_max=0 likewise.
CREATE TYPE promo_kind AS ENUM ('percent', 'fixed', 'free_shipping');

CREATE TABLE public.promo_code (
    code text PRIMARY KEY,
    kind promo_kind NOT NULL,
    value_minor bigint NOT NULL DEFAULT 0,
    currency text NOT NULL DEFAULT 'USD',
    valid_from timestamptz,
    valid_until timestamptz,
    max_uses int NOT NULL DEFAULT 0,
    per_customer_max int NOT NULL DEFAULT 1,
    used_count int NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- promo_redemption is the per-order ledger; the (code, order_id) PK gives us
-- idempotency on retries (the same order can't be redeemed twice) and a fast
-- lookup for per-customer limits via the (code, customer_id) index.
CREATE TABLE public.promo_redemption (
    code text NOT NULL REFERENCES public.promo_code(code) ON DELETE CASCADE,
    order_id text NOT NULL,
    customer_id text NOT NULL,
    discount_amount_minor bigint NOT NULL,
    currency text NOT NULL,
    redeemed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (code, order_id)
);

CREATE INDEX promo_redemption_customer_idx
    ON public.promo_redemption(code, customer_id);

COMMIT;
