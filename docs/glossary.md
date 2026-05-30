# Glossary

> Domain Driven Design depends on a **ubiquitous language**: a shared
> vocabulary that the code, the tests, the docs and the team all use the
> same way. This file is that vocabulary.
>
> Each term is followed by a short definition, where it lives in the code,
> and any nuance that is easy to miss. Definitions reflect how the term is
> used **today**; if you change the meaning, change this file in the same
> PR.

See the top-level [README](../README.md#context-map) for the bounded-context
map, and the [ADRs](./adr/Readme.md) for the architecture decisions the
language sits on top of.

## How to use this glossary

Terms are listed alphabetically. The `[Context]` tag tells you which
bounded context owns the term — when a term spans contexts (`Address`,
`Status`) every owning context is listed and the differences are spelled
out in the entry. Cross-references use **See also:**. Terms that mean
different things in different contexts (`User` vs `Customer`,
`Product` vs `Variant` ids, etc.) are gathered under explicit
**Disambiguation** entries.

This is a working document. If you add a domain term to the code, add it
here in the same PR; if you change what an existing one means, fix the
definition here so the next reader is not misled.

## Terms

### Aggregate
A consistency boundary: a cluster of value objects whose invariants are
enforced through one root entity. This codebase has both flavours of
persistence:

- **Event-sourced**: the `Order` in checkout
  ([backend/checkout/domain/order.go](../backend/checkout/domain/order.go)).
  State is derived by folding the stream of [Domain events](#domain-event)
  through `apply()`. See also: [Event sourcing](#event-sourcing),
  [Snapshot](#snapshot).
- **State-stored**: `Fulfillment`
  ([backend/fulfillment/domain/fulfillment.go](../backend/fulfillment/domain/fulfillment.go)),
  `Cart` ([backend/cart/domain/cart.go](../backend/cart/domain/cart.go)),
  `Review` ([backend/reviews/domain/review.go](../backend/reviews/domain/review.go)).
  State lives as one row updated in place, guarded by an optimistic-
  concurrency `version` column where applicable.

Carrying both styles inside one project is intentional — the audit-heavy
order log earns event sourcing; the flat operational record of a shipment
does not.

### Anti-Corruption Layer (ACL) [cross-context]
A translation layer placed between two bounded contexts so that neither
leaks its types or semantics into the other. The reference example is
`transformProductCatalog` in
[backend/cart/bounded_context.go](../backend/cart/bounded_context.go),
which turns a `productcatalog.Variant` into the cart's own `domain.Product`
(re-checking stock and currency on the way through). The `reviews` context
uses a similar small port (`VerifiedBuyerSource`) to ask checkout whether
a customer actually bought a product without pulling the order aggregate
into reviews.

### Attribute set [productcatalog]
A named, reusable grouping that selects which [Attribute types](#attribute-type)
a product carries and in what order. Lives in
[backend/productcatalog/domain/attribute_set.go](../backend/productcatalog/domain/attribute_set.go);
the product references it by id via `Product.AttributeSetID()`. A product
with no set falls back to all attribute types.

### Attribute type [productcatalog]
The *definition* of a filterable product attribute (the schema). Either
`numeric` (a number with an optional unit, e.g. "Weight, kg") or `enum`
(a value drawn from a fixed set, e.g. "Material"). Marked `filterable` if
it should appear in the listing-page facet panel. Defined in
[backend/productcatalog/domain/attribute.go](../backend/productcatalog/domain/attribute.go)
as `AttributeType`.

### Attribute value [productcatalog]
A specific product's value for one [Attribute type](#attribute-type)
— `AttributeValue` in
[backend/productcatalog/domain/attribute.go](../backend/productcatalog/domain/attribute.go).
Numeric values populate `numValue`; enum values populate `textValue`. The
display helper picks the right rendering and appends the unit when set.

### Bounded context
A model boundary inside the codebase: one team's view of the domain, with
its own vocabulary, its own data, and a deliberately narrow surface area
to the others. The backend ships these contexts (each under
`backend/<name>/` with `domain`, `app`, `adapter`, `bounded_context.go`):

`auth`, `cart`, `checkout`, `fulfillment`, `productcatalog`, `promo`,
`reviews`, `search`, `shippinginfo`, `store`, `wishlist`, plus the
presentation context `layout` (HTMX storefront + admin panel).

See the [context map in the top-level README](../README.md#context-map).

### Cart vs Order [Disambiguation]
A **Cart** is an open, mutable collection of variants tied to a single
session ([backend/cart/domain/cart.go](../backend/cart/domain/cart.go)) —
items can be added and removed at any time, and it has no concept of
"placed".

An **Order** is the immutable snapshot created when the customer checks
out ([backend/checkout/domain/order.go](../backend/checkout/domain/order.go)).
Its `Line`s freeze the price and product name at order time; later
catalogue changes do not retroactively change a placed order.

The two live in different bounded contexts and share no Go types — the
checkout context reads the cart through a narrow interface at order time
and from then on the order is self-contained.

### Channel [checkout]
The sales channel an `Order` was placed through ("web", "ios", "api").
Carried on `OrderPlaced` v2 events; v1 rows are upcast to "unknown" by
the codec (see [Upcaster](#upcaster)). Today every producer passes "web",
but the parameter is wired through so a future iOS / API entry point
needs no schema change.

### Conformist [cross-context]
A strategic-design relationship where the downstream context accepts the
upstream's model and vocabulary as-is, without a translation layer. Cheap
but tightly coupled: any upstream change ripples through. Here: `layout`
declares narrow per-context interfaces but those interfaces refer to each
producer's domain types directly (e.g. `pcdomain.Product`,
`checkoutDomain.Order`, `fulfillmentDomain.Fulfillment`); see the imports
at the top of
[backend/layout/bounded_context.go](../backend/layout/bounded_context.go).
A presentation layer that only renders is a natural conformist. Contrast
with [Anti-Corruption Layer](#anti-corruption-layer-acl-cross-context).

### Customer [auth]
An authenticated end user, identified by email address. The aggregate is
`Customer` in [backend/auth/domain/customer.go](../backend/auth/domain/customer.go).
Sessions reference a customer through `Session.CustomerID()`. The
cart's `User` is **not** a customer — see [User vs Customer](#user-vs-customer-disambiguation).

### Customer-Supplier [cross-context]
A strategic-design relationship where two contexts collaborate on the
shape of their seam: the downstream "customer" tells the upstream
"supplier" what it needs, and the supplier honours that contract. Here:
`checkout` (customer) defines `CartReader` and the stock ports
(`StockReserver`, `StockMovements`) in
[backend/checkout/bounded_context.go](../backend/checkout/bounded_context.go);
`cart` and `productcatalog` (suppliers) implement them. The contract is
checkout-shaped, not a generic catalogue API. Distinct from
[Conformist](#conformist-cross-context) (no negotiation) and from
[Anti-Corruption Layer](#anti-corruption-layer-acl-cross-context) (one-way
translation against an unchangeable upstream).

### Discount [promo]
The *resolved* effect of a promo [Code](#code) on one specific order:
the amount in minor units actually subtracted (or a `freeShipping` flag
for the free-shipping kind). Produced by `Code.Apply(subtotal, shipping)`
in [backend/promo/domain/code.go](../backend/promo/domain/code.go) and
fed into the checkout pricing math. Distinct from `Code`, which is the
catalogue rule.

### Code [promo]
The admin-created catalogue entry that defines a promo: the literal text,
its `Kind` (`percent`, `fixed`, `free_shipping`), the value, the validity
window and the usage caps. Lives in
[backend/promo/domain/code.go](../backend/promo/domain/code.go). Resolving
a code against a cart produces a [Discount](#discount).

### Domain event [checkout, fulfillment]
A fact about an aggregate's state change, raised inside the publishing
context and never carried outside it. Examples:
[`OrderPlaced`, `PaymentSucceeded`, `OrderShipped`, …](../backend/checkout/domain/events.go)
on the checkout `Order`;
[`FulfillmentScheduled`, `FulfillmentLabeled`, …](../backend/fulfillment/domain/fulfillment.go)
on the fulfillment process manager. Domain events are the source of
truth for an event-sourced aggregate; for a state-stored one they are
only an in-memory bridge to the bus during command execution.

Contrast with [Integration event](#integration-event).

### Event sourcing [checkout]
The persistence pattern where an aggregate's state is derived from a
sequence of immutable events appended to an event log, rather than read
from a row. Used only by the `Order` aggregate
([backend/checkout/domain/order.go](../backend/checkout/domain/order.go)
+ [backend/checkout/adapter/events_postgres.go](../backend/checkout/adapter/events_postgres.go)).
Every other aggregate in this codebase is state-stored.

See also: [Snapshot](#snapshot), [Upcaster](#upcaster),
[Projection](#projection--read-model).

### Fulfillment [fulfillment]
The operational lifecycle of a paid order: `scheduled → labeled →
shipped → delivered`, with a `refunded` branch reachable from any
non-terminal active state. Lives in its own bounded context as a
[Process Manager](#process-manager); see
[backend/fulfillment/domain/fulfillment.go](../backend/fulfillment/domain/fulfillment.go).

Deliberately distinct from the *commercial* state on the checkout
`Order` aggregate (`pending → paid / failed / cancelled`). Two
lifecycles, two state machines, two stores: finance owns one,
the warehouse owns the other.

### Hit [search]
A single search result row: a [Document](#document) plus a relevance
`Rank` (`Hit` in [backend/search/app/service.go](../backend/search/app/service.go)).
The search context does not define what "rank" means beyond "higher is
better"; the Postgres adapter populates it with `ts_rank_cd`. Consumers
are free to ignore `Rank`.

### Document [search]
The published-language value object of the search [OHS](#open-host-service-ohs):
a kind + id + title + body + url + tags + meta + timestamp. Producers
translate their own records to a `Document` on their side and call
`Indexer.Index`. Defined in
[backend/search/domain/document.go](../backend/search/domain/document.go).
The set of valid `Kind` strings is intentionally open (any non-empty
string), so a new producer adds itself without changing the search
context.

### Integration event [cross-context]
An event published by one context for *other* contexts to react to. The
shape lives in the publishing context's `integration/` package and is
the stable, public contract — e.g.
[`checkout.OrderPaid`](../backend/checkout/integration/events.go).
Carried in-process by the [event bus](#event-bus) and made durable by
the [Transactional Outbox](#transactional-outbox).

Today's integration events are **notification events**: they carry IDs
only (`OrderID`, `SessionID`, `CustomerID`), and any subscriber that
needs more queries back through the published context's read side. The
alternative — Event-Carried State Transfer, where the event carries the
full state — is not used here.

### Event bus [internal]
A tiny synchronous, in-process publish/subscribe dispatcher
([backend/internal/eventbus/eventbus.go](../backend/internal/eventbus/eventbus.go)).
Publishers don't know who (if anyone) subscribes; handler errors are
logged but never abort the publish. The bus is the *delivery* mechanism
for [Integration events](#integration-event); the producers stage them
into the [Transactional Outbox](#transactional-outbox) first so a crash
between commit and publish doesn't lose them.

### Transactional Outbox [internal]
The pattern (and the table) that guarantees at-least-once delivery of
[Integration events](#integration-event). Producers write outbox rows
in the SAME database transaction as the aggregate's state change; a
separate `outbox.Dispatcher`
([backend/internal/outbox/dispatcher.go](../backend/internal/outbox/dispatcher.go))
polls unsent rows on a ticker and publishes them onto the
[event bus](#event-bus). The pattern's value here is its *shape*
(durable hand-off, idempotent subscribers) rather than throughput.

Because delivery is at-least-once, subscribers must be idempotent:
`fulfillment.Schedule` rejects a second `OrderPaid` for the same order
with `ErrAlreadyExists` for exactly this reason.

### Open Host Service (OHS) [search]
A context's deliberately stable, public API that any other context can
integrate with — instead of producer-specific adapters per consumer.
The `search` context is the in-repo example: it publishes the
[Document](#document) value object plus two ports (`Indexer` for
writers, `Querier` for readers) from
[backend/search/app/service.go](../backend/search/app/service.go).
Productcatalog talks to it today; a blog or FAQ producer could light
up the same surface with no changes to search.

### Order vs OrderSummary vs OrderView [Disambiguation]
The three faces of the checkout context, illustrating [CQRS](#projection--read-model)
in action.

- `Order` — the write-side, event-sourced aggregate in
  [backend/checkout/domain/order.go](../backend/checkout/domain/order.go).
  Receives commands (`PlaceOrder`, `MarkPaid`, `Cancel`, …) and raises
  events.
- `OrderSummary` — the read model for list pages (account orders, admin
  order list). Defined in
  [backend/checkout/query/views.go](../backend/checkout/query/views.go).
- `OrderView` — the read model for the order-detail page. Same file.

Read models are projected from the event stream and are never used to
issue commands.

### Partnership [cross-context]
A strategic-design relationship where two contexts jointly evolve their
shared interface, succeeding or failing together. The strongest form of
coupling between two otherwise distinct contexts, and rare in practice.
Not realised in this codebase today — no two contexts here co-own a
contract; every cross-context seam is either
[Customer-Supplier](#customer-supplier-cross-context),
[ACL](#anti-corruption-layer-acl-cross-context),
[OHS](#open-host-service-ohs) /
[Published Language](#published-language-cross-context), or
[Conformist](#conformist-cross-context).

### Process Manager [fulfillment]
A long-running coordinator that reacts to events from other aggregates
and emits commands of its own. The `fulfillment` context is the example
([backend/fulfillment/domain/fulfillment.go](../backend/fulfillment/domain/fulfillment.go)):
it subscribes to `checkout.OrderPaid`, spawns a `Fulfillment` in
`StatusScheduled`, and is then driven forward by warehouse commands.

Distinct from a **saga** (which usually defines compensating actions),
from an **aggregate** (which owns state but not orchestration), and
from the `Order` aggregate that emitted the triggering event.

### Product vs Variant ids [Disambiguation]
A `Product`
([backend/productcatalog/domain/product.go](../backend/productcatalog/domain/product.go))
is the catalogue entry. A `Variant`
([backend/productcatalog/domain/variant.go](../backend/productcatalog/domain/variant.go))
is one purchasable combination of [Option type](#option-type) values
with its own price, stock and SKU. A product with no options still has
a single default variant.

**Cart and order line items reference *variant* ids, not *product* ids.**
This is a frequent source of confusion — a wishlist item also keys on
variant id, and the stock-reservation system operates on variants. The
only place product ids cross context boundaries is the [Review](#review-1)
(reviews are per product, not per variant).

### Option type [productcatalog]
A product attribute the customer chooses at purchase time, e.g. "Color"
with values `["Red","Blue"]`. Defined in
[backend/productcatalog/domain/variant.go](../backend/productcatalog/domain/variant.go)
as `OptionType`. A `Product` has zero or more option types; each
`Variant` is one combination of option-type values.

### Published Language [cross-context]
A documented, stable interchange schema that one or more contexts agree
to read or produce — the "language" of the conversation between them,
deliberately decoupled from any one context's internal model. Here:
[`checkout.OrderPaid`](../backend/checkout/integration/events.go) is the
checkout context's outward shape; the cart, fulfillment and email
subscribers consume it through the [event bus](#event-bus) +
[Transactional Outbox](#transactional-outbox) without checkout knowing
they exist. The search [Document](#document) value object is a second
example, paired with the search [OHS](#open-host-service-ohs). Closely
related to [Integration event](#integration-event), which is the
specific carrier here.

### Projection / Read model [checkout]
The CQRS read side: a flat representation of an aggregate, derived from
its events, tuned for queries. The checkout context maintains two
projections off the same event stream:

- The **order read model** (`OrderSummary` + `OrderView`), projected by
  [backend/checkout/adapter/events_postgres.go](../backend/checkout/adapter/events_postgres.go).
- The **analytics_daily_sales** projection — a per-currency tally of
  paid orders, surfaced as
  [`query.DailySalesRow`](../backend/checkout/query/views.go).

The two readers share an event source but never share rows or rebuild
logic, demonstrating that a single write stream can fan out to multiple
independent read models.

### Reservation (Stock reservation) [productcatalog, checkout]
Stock decremented at order-place time, before payment is confirmed. The
audit row lives in
[backend/productcatalog/domain/stock_movement.go](../backend/productcatalog/domain/stock_movement.go)
(`StockMovement` with a negative `Delta` and a `RefOrderID`). If the
order fails to finalise, the [Reservation sweeper](#reservation-sweeper)
expires the pending order and releases the reservation. The opposite
direction — `OrderRefunded` — also adds stock back via a positive delta.

### Reservation sweeper [checkout]
The periodic background worker
([backend/checkout/sweeper/sweeper.go](../backend/checkout/sweeper/sweeper.go))
that finds pending orders older than a configurable TTL and runs them
through `ExpirePending`, releasing the held [Reservation](#reservation-stock-reservation).
Without it, a crash or a hung async payment between `OrderPlaced` and
`PaymentSucceeded`/`PaymentFailed` would hold stock forever.

### Review [reviews]
A customer-submitted rating (1–5) and body on a `Product`. One review
per (customer, product). Lives in
[backend/reviews/domain/review.go](../backend/reviews/domain/review.go)
with a moderation lifecycle `pending → approved | rejected` and a
soft-delete escape hatch (the unique index is partial so a buyer can
re-review after a removal). Only verified buyers can submit — see
[Verified buyer](#verified-buyer).

### Shared Kernel [cross-context]
A strategic-design relationship where two contexts deliberately share a
small, jointly-owned set of types — typically primitives like `Money` or
`Currency`. Any change to the kernel is a coordinated change across both
sides, so the kernel is kept intentionally tiny. Not realised in master
today: each context defines its own currency / money types and the cart's
ACL re-validates them on the way through. Listed here so a future `Money`
value object lifted into `backend/internal/` lands with a name.

### Snapshot [checkout]
A serialised point-in-time copy of an event-sourced aggregate's state,
written every N events to cap replay cost.
[`OrderSnapshot`](../backend/checkout/domain/snapshot.go) mirrors the
`Order` aggregate one-to-one as a public DTO; the adapter (un)marshals
it as JSON and `RehydrateOrderFromSnapshot` seeds the aggregate then
applies the tail of events since the snapshot. Snapshots are an
optimisation, not a new truth — replaying from event 0 yields the
identical aggregate.

### Specification [promo]
A small, composable predicate encapsulating one business rule. The
promo eligibility check is the in-repo example
([backend/promo/app/spec.go](../backend/promo/app/spec.go)): each rule
is a struct with `IsSatisfiedBy(EligibilityContext) error`, composed
via `And(...)` which short-circuits on the first failure. The four
rules:

- `NotAnonymous` — code redemptions require a logged-in customer.
- `WithinValidityWindow` — `Code` must be active at `Now`.
- `UnderMaxUses` — global redemption cap not yet hit.
- `UnderPerCustomerLimit` — this customer hasn't exhausted their cap.

Returning an `error` rather than a `bool` is intentional: the caller
needs to know *which* rule rejected the code so it can render the
right flash message.

### Status [checkout, fulfillment, reviews]
A bounded `string` type used as a small state machine. Three meanings
in three contexts, **never** interchangeable:

- `checkout.domain.Status` — commercial state of an `Order`:
  `pending → paid → cancelled | shipped → delivered → refunded`.
- `fulfillment.domain.Status` — operational state of a `Fulfillment`:
  `scheduled → labeled → shipped → delivered | refunded | returned`.
- `reviews.domain.Status` — moderation state of a `Review`:
  `pending → approved | rejected`.

Both `checkout` and `fulfillment` define a `StatusShipped` /
`StatusDelivered` — they happen to align in spelling because the same
real-world event (carrier handover, doorstep drop) advances both
machines, but they belong to different aggregates and different
transactions.

### Store [store]
A storefront facade identified by Host header, with its own slug, name
and display currency. Lives in
[backend/store/domain/store.go](../backend/store/domain/store.go); the
active store is resolved per request by `storeMiddleware` and threaded
onto the request context. Distinct from "storage" (the term used in the
`adapter` packages for the persistence interface).

### Upcaster [checkout]
The function that translates an old version of a domain event payload
to the latest version at load time. The aggregate's `apply()` only
ever sees the latest shape. The in-repo example is
`upcastOrderPlacedV1` in
[backend/checkout/adapter/events_codec.go](../backend/checkout/adapter/events_codec.go),
which fills `Channel = "unknown"` on v1 `OrderPlaced` rows that
predate the v2 [Channel](#channel) field.

### User vs Customer [Disambiguation]
- `cart.domain.User` — a session identifier
  ([backend/cart/domain/user.go](../backend/cart/domain/user.go)). It
  is just the `cart_id` cookie value; anonymous shoppers have one too.
- `auth.domain.Customer` — an authenticated end user identified by
  email ([backend/auth/domain/customer.go](../backend/auth/domain/customer.go)).

They are **not** the same thing and never share an id. An `Order`
carries both: `UserID` (the session, always present) and `CustomerID`
(empty for guest checkouts; populated only for logged-in customers,
who then see the order in their account history).

### Verified buyer [reviews]
A `Customer` with at least one paid / shipped / delivered order
containing the `Product` they want to review. The reviews context
exposes a `VerifiedBuyerSource` ACL port
([backend/reviews/bounded_context.go](../backend/reviews/bounded_context.go))
that the composition root satisfies by joining `checkout_order_item`
to `productcatalog_variant.product_id` — so reviews never imports
checkout's types.

### Wishlist item [wishlist]
A `(customer, variant, added_at)` tuple
([backend/wishlist/domain/item.go](../backend/wishlist/domain/item.go)).
Keyed on **variant id**, not product id, so a customer can wishlist
"the red large mug" independently of "the blue small mug". See also:
[Product vs Variant ids](#product-vs-variant-ids-disambiguation).

### Saved address [shippinginfo]
A shipping address in a customer's address book
([backend/shippinginfo/domain/address.go](../backend/shippinginfo/domain/address.go))
— an entity with identity and a mutable `isDefault` flag. Contrast with
`checkout.domain.Address`
([backend/checkout/domain/address.go](../backend/checkout/domain/address.go)),
which is an immutable per-order value-object snapshot. The checkout
context never reads from `shippinginfo`; the layout layer hands the
chosen saved address to the checkout form, which then constructs its
own `Address` value object at order time.
