# Shared Kernel

This package is the project's **Shared Kernel** in the DDD sense: a small,
deliberately stable common language that multiple bounded contexts depend on.
The kernel is *published*: changing it costs coordination across every
consuming context, so the bar for adding types here is high and the bar for
changing existing ones is higher.

## Contents

Today the kernel ships a single type:

- **`Money`** — an `(int64 minor units, ISO 4217 currency)` value object with
  currency-aware arithmetic. The pair was previously threaded around as two
  parameters (`amount int64`, `currency string`), which let cross-currency
  bugs slip through type checks. `Money.Add`, `Sub` and `Compare` return
  `ErrCurrencyMismatch` when the operands disagree; `Mul` and `MulFloat`
  preserve the currency. `MulFloat` rounds via `math.Round` and is the
  designated hook for percent math (tax, discount).

The full contract is documented on the type itself in `money.go`.

## Migration plan

Adoption is **gradual**. The shared kernel exists from day one, but it does
not need to land in every context at the same time:

- **Today (this PR):** the `checkout` context exposes Money-returning getters
  *alongside* its existing `(int64, string)` accessors on `Order`, `OrderView`
  and `OrderSummary`. The pricing domain service (`PriceQuote`) still computes
  in `int64`; the `Quote` value object gains Money-returning helpers but keeps
  its int64 fields. Storage and events stay `int64`-based.
- **Next:** `cart` and `productcatalog` adopt Money on their read sides, then
  expose Money on commands.
- **Later:** events and persistence rows migrate to store Money. Until that
  ships, contexts that have not yet adopted Money exchange `(int64, string)`
  at their boundaries and convert at the edge — typically a single
  `sharedkernel.NewMoney(amt, sharedkernel.Currency(ccy))` call.

The kernel deliberately does NOT include FX conversion: that is an
infrastructure concern (rates, providers, caching) and belongs in its own
service when the business needs it.
