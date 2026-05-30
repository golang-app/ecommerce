# eventcatalog

A hand-curated registry of every event the codebase emits, plus the
small `ecommerce events` CLI subcommand that renders it as Markdown.

## Why hand-curated?

The codebase has two flavours of events:

- internal **domain** events inside aggregates (event-sourced in
  `checkout/domain`, or in-memory bridges in `fulfillment/domain`);
- **integration** events published on the bus as the published
  language of one bounded context to its subscribers
  (`checkout/integration`, `fulfillment/integration`).

A real AST walker could discover them, but it would have to track our
conventions for declaring an event (the `EventType`/`EventName`
methods, the marker interfaces, codec version switches) and stay
correct as those conventions evolve. A flat slice of `Event` literals
is cheap to read, trivial to review, and orders of magnitude easier
to keep accurate. The discipline is the same one the
`docs/glossary.md` follows: when you introduce a new event, add a row
in the same commit.

The unit test `TestCatalogIsConsistent` enforces the basic shape (no
duplicate `Name`s, valid `Kind`, `Version >= 1`, no empty `Name` /
`Kind` / `Package` / `Producer`) so accidental drift fails CI.

## Adding a new event

When you introduce a new event type:

1. Add the Go type and its `EventType()` / `EventName()` method in
   the producing context (e.g. `checkout/domain` or
   `fulfillment/integration`).
2. Append a row to `Catalog()` in `catalog.go` describing it. Pick
   a `Kind` (`KindDomain` or `KindIntegration`), set `Package` to
   the relative import slice (e.g. `"checkout/domain"`), set
   `Producer` to the bounded context name, set `Version` to `1`,
   and list known `Consumers` (may be empty).
3. Write a one-line `Description`. Note whether the event is
   notification-style (IDs only) or event-carried-state-transfer
   (full snapshot) when it is non-obvious.
4. Run the verification gate: `go test ./internal/eventcatalog/...`
   plus `go run ./cmd/cli events` to eyeball the rendered table.

## Changing an event's schema

When you change an event's payload shape (add or remove a field, or
re-interpret an existing one):

1. Bump the `Version` in `Catalog()` to the new latest value the
   writer emits.
2. Wire the version into the codec's marshal / unmarshal switch in
   the owning context's adapter (see
   `checkout/adapter/events_codec.go` for the OrderPlaced v1 -> v2
   upcaster as the worked example).
3. Note the change in the `Description` so the rendered table tells
   readers why the version bumped (one short sentence is plenty).

## How to view the catalog

```sh
go run ./cmd/cli events             # default: one Markdown table, sorted by Name
go run ./cmd/cli events --by-context  # one H2 per producing context
```

The output is plain stdout — pipe it into a file or into your
favourite Markdown previewer.
