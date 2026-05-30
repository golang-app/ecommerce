# Storage conformance tests

This codebase pairs every adapter: a postgres implementation backs
production and an in-memory implementation backs fast unit tests. When the
two drift the bug hides in plain sight — a fix lands in the in-memory store
the integration tests never exercise, or vice-versa.

A **storage conformance test** fixes that. One table-driven suite per
`Storage` interface runs against BOTH adapters. One bug, one fix, one
place; [Liskov substitution](https://en.wikipedia.org/wiki/Liskov_substitution_principle)
as enforceable code.

## Anatomy of a suite

Next to the `Storage` interface (in the same `app` package so the suite
can use the unexported sentinel errors) add a `conformance.go`:

```go
// RunStorageConformance exercises every contract guarantee of Storage.
// Adapters call it from their own test files with a factory that hands
// back a freshly initialised store.
func RunStorageConformance(t *testing.T, newStorage func() Storage) {
    t.Helper()

    t.Run("Create_RoundTripsViaFind", func(t *testing.T) { ... })
    t.Run("Update_OverwritesPersistedState", func(t *testing.T) { ... })
    // ...one sub-test per contract clause...
}
```

Each adapter then ships a thin test file that supplies the factory:

```go
// promo/adapter/inmemory_test.go         (default build tag)
func TestInMemory_Conformance(t *testing.T) {
    promoapp.RunStorageConformance(t, func() promoapp.Storage {
        return NewInMemory()
    })
}

// promo/adapter/postgres_test.go         (//go:build integration)
func TestPostgres_Conformance(t *testing.T) {
    db := openTestDB(t)
    promoapp.RunStorageConformance(t, func() promoapp.Storage {
        wipePromoTables(t, db)
        return NewPostgres(db)
    })
}
```

Both files invoke the SAME suite. New contract tests added to the suite
apply to both adapters automatically — that is the entire point.

## Where the suite lives

In the `app` package, not the `adapter` package, because:

- it needs `Storage` and the sentinel errors (`ErrCodeNotFound`, …);
- both adapter test files import it, so it must not depend on either
  adapter;
- placing it in `app` (rather than `app_test`) makes it importable
  from `package adapter` test files without an import cycle.

The function takes `*testing.T` from `testing` — a tiny import in
production code, in exchange for one suite that runs against every
adapter.

## Conventions

- Each sub-test calls `newStorage()` once, so adapters that share state
  between sub-tests can wipe it cleanly per case.
- Postgres factories sit behind `//go:build integration` and call a local
  table-wipe helper before returning the adapter.
- Sub-test names follow `Verb_What` (`Create_RoundTripsViaFind`,
  `Redeem_AtomicUnderConcurrency`) so a single `go test -run` regex can
  target one clause across both adapters.
- Concurrency tests use `t.Parallel` + a `sync.WaitGroup` barrier to prove
  atomicity rather than relying on race-detector luck.

## Adopt the pattern in a new context

1. Decide what contract guarantees the `Storage` interface makes — read
   the application service to see what it relies on.
2. Add `conformance.go` next to the interface with one sub-test per
   guarantee.
3. Add `adapter/inmemory_test.go` (default build) and
   `adapter/postgres_test.go` (`//go:build integration`) that each call
   `RunStorageConformance`.
4. Delete any drift-prone duplicated adapter tests that the suite now
   covers.

The in-repo exemplar is the **promo** context — see
[`backend/promo/app/conformance.go`](../../promo/app/conformance.go) and
the two adapter test files alongside `inmemory.go` / `postgres.go`.
