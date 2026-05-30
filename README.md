# go-ecommerce

GoCommerce is an e-commerce application written in Go and HTMX. The goal of this project is to show good practices and examples of some domain or technical decisions.

The whole application is split into a few major parts:

* [backend](./backend) - the backend implementation that exposes an API for the frontend with frontend written in HTMX
* [docs](./docs) - documentation that's more high-level (ADRs, architecture diagrams, etc)

If you find anything that you can improve or add - feel free to talk about it in the [discussions](https://github.com/bkielbasa/go-ecommerce/discussions) or create a [pull request](https://github.com/bkielbasa/go-ecommerce/pulls).

The project is a very early stage so there's a lot of work to do so every contribution is welcome!

## Features

**Storefront**
- "New arrivals" homepage; full filterable catalog at `/products`.
- Top menu with category links, cart and account/log out.
- Faceted filters per category (numeric ranges + enum checkboxes), scoped to the active category.
- Variant selectors (e.g. Color / Size) on product pages; add-to-cart over HTMX.
- Customer accounts: orders history, saved shipping addresses, password change.
- Checkout with personal pickup / courier / flat-rate shipping and card / PayPal / cash-on-delivery payment.

**Admin panel** (`/admin`, seeded user `admin@example.com` / `Admin123!`)
- Dashboard with store counts.
- Products: list, create (simple or with option types + variants), edit core fields, per-variant SKU/price/stock/image, add/edit/delete variants, manage option types on existing products (with consistent cascades), assign categories, attributes and an attribute set, delete.
- Categories, attribute types and attribute sets: full CRUD.
- Orders: list all, view detail, admin-cancel.
- Dedicated admin shell (sidebar layout) separate from the storefront.

## Context map

The backend is organised as a set of DDD bounded contexts. Each context owns its
data and is split into `domain` (entities, value objects, invariants), `app`
(application services / use cases), and `adapter` (persistence — a Postgres and
an in-memory implementation), with a thin `bounded_context.go` wiring it together.
The `layout` context is the presentation layer (HTMX storefront + admin panel);
it depends on the others through small, locally-defined interfaces.

```mermaid
graph TD
    layout["layout<br/><i>presentation: storefront + admin (HTMX)</i>"]
    productcatalog["productcatalog<br/><i>products, variants, option types,<br/>attributes, categories, stock</i>"]
    cart["cart<br/><i>shopping carts</i>"]
    checkout["checkout<br/><i>orders — event-sourced + CQRS</i>"]
    auth["auth<br/><i>customers, sessions, admin role</i>"]
    shippinginfo["shippinginfo<br/><i>saved addresses</i>"]

    layout -->|Conformist| productcatalog
    layout -->|Conformist| cart
    layout -->|Conformist| checkout
    layout -->|Conformist| auth
    layout -->|Conformist| shippinginfo

    cart -->|ACL: variant → cart product| productcatalog
    checkout -->|Customer-Supplier: cart read port| cart
    checkout -->|Customer-Supplier: stock ports| productcatalog
    checkout -. "Published Language: OrderPaid (bus)" .-> cart
```

### Relationship types

Each edge above is labelled with one of the standard DDD strategic-design
relationships. The patterns realised in the code are:

- **Anti-Corruption Layer (ACL)** — a translation layer that protects one
  context from another's vocabulary. Here: `cart` translates a
  `productcatalog.Variant` into its own `domain.Product` (re-checking stock
  and currency) in
  [`transformProductCatalog`](./backend/cart/bounded_context.go) — cart
  never imports productcatalog types beyond that seam.
- **Customer-Supplier** — two contexts collaborate on a port the customer
  needs; the supplier agrees to honour it. Here: `checkout` defines
  `CartReader` and the stock ports (`StockReserver`, `StockMovements`) in
  [`backend/checkout/bounded_context.go`](./backend/checkout/bounded_context.go);
  `cart` and `productcatalog` implement them. The contract is checkout-shaped,
  not a generic catalogue API.
- **Published Language** — a documented, stable interchange schema. Here:
  the integration event `checkout.OrderPaid`
  ([`backend/checkout/integration/events.go`](./backend/checkout/integration/events.go))
  is checkout's outward-facing shape — subscribers (cart, fulfillment, the
  email subscriber) consume it through the in-process bus + Outbox without
  checkout knowing they exist.
- **Conformist** — the consumer accepts the producer's vocabulary with no
  translation. Here: `layout` declares narrow per-context interfaces but
  refers to each producer's domain types directly (e.g. `pcdomain.Product`,
  `checkoutDomain.Order`, `fulfillmentDomain.Fulfillment`); see the imports
  at the top of
  [`backend/layout/bounded_context.go`](./backend/layout/bounded_context.go).
  A presentation layer that just renders is a natural conformist.
- **Open Host Service (OHS)** — a context exposes a deliberately stable,
  public API any other context can integrate with. Here: `search` publishes
  the `Document` value object plus the `Indexer` / `Querier` ports in
  [`backend/search/app/service.go`](./backend/search/app/service.go).
  Productcatalog is the in-repo producer today (via the
  `SearchIndexer` port), but the surface is intentionally generic — a blog
  or FAQ producer could light it up with no changes to search. (Not drawn
  in the diagram above because the search context is omitted for
  readability; see the table below.)

The other patterns from Evans' catalogue — **Partnership** (joint
co-evolution) and **Shared Kernel** (a small set of shared types) — are
not realised in master today, so no edge above carries those labels.

| Context | Responsibility | Talks to |
| --- | --- | --- |
| **productcatalog** | Products, variants, option types, filterable attributes, categories, and stock reservation. No dependencies on other contexts. | search (OHS producer side: publishes Documents via `SearchIndexer`) |
| **cart** | Session-scoped shopping carts. Resolves a variant id into its own product notion through an anti-corruption layer over productcatalog. | productcatalog (sync, ACL); checkout (async, subscribes to `OrderPaid` to clear the basket) |
| **checkout** | Orders as an event-sourced aggregate with a separate CQRS read side. Snapshots the cart, reserves stock, records payment, and publishes an `OrderPaid` integration event. | cart (sync, Customer-Supplier on `CartReader`); productcatalog (sync, Customer-Supplier on stock ports); downstream subscribers via Published Language `OrderPaid` |
| **auth** | Customers, sessions, password policy, and the admin role. Standalone. | — |
| **shippinginfo** | Customers' saved shipping addresses. Standalone. | — |
| **search** | Free-text + filtered search over an open set of `Document` kinds. Two ports: `Indexer` (write) and `Querier` (read). | productcatalog (OHS consumer side) |
| **layout** | HTTP presentation: the HTMX storefront and the admin panel. Orchestrates every context through narrow interfaces, conforming to each producer's domain types. | all contexts (Conformist) |

Cross-context integration is mostly synchronous through anti-corruption interfaces
defined at the composition root (`backend/cmd/web/main.go`). The one decoupled
path is an in-process event bus (`backend/internal/eventbus`): checkout publishes
`OrderPaid` after a successful payment and the cart context subscribes to empty
the basket, so checkout never calls the cart directly for that side effect.

The shared vocabulary used across these contexts is collected in the
[ubiquitous-language glossary](./docs/glossary.md). See
[docs/glossary.md](./docs/glossary.md) for the full DDD vocabulary used
by this project — including the relationship-type definitions above.


## Quick start

The easiest way of running everything is using the `docker-compose`.

```sh
docker-compose up
```

You'll have to wait some time to download all dependencies and build everything but after it, everything should be up and running.
