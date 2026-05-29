# search — Open Host Service

The `search` package publishes a single value object — `domain.Document` —
together with a `Kind` discriminator. Any other bounded context that owns
searchable content (today: `productcatalog`; tomorrow: `blog`, `faq`) maps
its records into that published language via a small Anti-Corruption Layer
on the producer side and calls the `Indexer` port to keep the index in
sync.

This is the **Open Host Service (OHS)** pattern from DDD: the search context
exposes a stable, narrow contract (the `Document` shape + two ports) and
every consumer translates into it. Producers do **not** depend on search's
storage or app types; they depend only on `search/domain`.

## Ports

- `app.Indexer` — write side, used by producers: `Index(ctx, Document)`,
  `Remove(ctx, kind, id)`.
- `app.Querier` — read side, used by consumers (the storefront):
  `Search(ctx, q, QueryOptions) ([]Hit, error)`.
- `app.Storage` — persistence port the adapters satisfy
  (postgres + in-memory).

The `*app.Service` struct implements **both** roles, so production wires the
same instance into productcatalog's `SearchIndexer` slot and layout's
`searchService` slot.

## Storage shape

Postgres (migration `000028_search_documents`): a single `search_document`
table keyed by `(kind, id)` with a stored `tsvector` aggregating title
(weight A), body (weight B) and tags (weight C), backed by a GIN index.
Queries go through `websearch_to_tsquery('simple', $1)` and rank with
`ts_rank_cd`.

The `simple` configuration is used (not `english`) to keep substring-like
behaviour for short product titles; switching to `english` (stemming) is a
one-migration change.

## Adding a new producer (e.g. `blog`)

1. In your producer package, declare your own `Kind` constant (any
   non-empty string — search does not maintain a closed enum).
2. Declare a local `SearchIndexer` interface mirroring `app.Indexer` (so
   your package does not import `search/app`).
3. Write a translator (`postToDocument`) that returns a
   `search/domain.Document`.
4. Call `Index` / `Remove` after every successful mutation; log and
   continue on failure (the index can be rebuilt by `cmd/cli reindex`).
5. Add a `reindex <yourkind>` subcommand if you want offline rebuilds.

No change to the `search` package is required.
