// Package storagetest documents the storage-conformance test pattern used
// by this codebase and hosts cross-context helpers for it. It deliberately
// holds no Go types today: each bounded context owns its own Storage
// interface and so each owns its own conformance suite. What lives here is
// the shape — the convention — so a reader can find one canonical place
// that explains why every adapter test file looks the way it does.
//
// # The pattern in one paragraph
//
// Every adapter in this codebase is paired: a postgres implementation for
// production and an in-memory implementation for fast unit tests. When the
// two drift, bugs leak between layers — a fix lands in the in-memory store
// the integration tests never exercise, or vice-versa. A storage
// conformance test fixes that by defining ONE table-driven suite per
// Storage interface and running it against BOTH adapters. One bug, one
// fix, one place; Liskov substitution as enforceable code.
//
// # How a conformance suite looks
//
// Next to the Storage interface (in the same `app` package so it can refer
// to the unexported error sentinels and the interface itself) add a file
// like `conformance.go`:
//
//	// RunStorageConformance exercises every contract guarantee of Storage.
//	// Adapters call it from their own test files with a factory that
//	// hands back a freshly initialised store.
//	func RunStorageConformance(t *testing.T, newStorage func() Storage) {
//	    t.Helper()
//	    t.Run("Create_RoundTripsViaFind", func(t *testing.T) { ... })
//	    t.Run("Update_OverwritesPersistedState", func(t *testing.T) { ... })
//	    // ...one sub-test per contract clause...
//	}
//
// Each adapter then has a thin test file that supplies its factory:
//
//	// promo/adapter/inmemory_test.go (default build tag)
//	func TestInMemory_Conformance(t *testing.T) {
//	    promoapp.RunStorageConformance(t, func() promoapp.Storage {
//	        return NewInMemory()
//	    })
//	}
//
//	// promo/adapter/postgres_test.go (//go:build integration)
//	func TestPostgres_Conformance(t *testing.T) {
//	    promoapp.RunStorageConformance(t, func() promoapp.Storage {
//	        wipePromoTables(t, db)
//	        return NewPostgres(db)
//	    })
//	}
//
// Both files invoke the SAME suite. New contract tests added to the suite
// apply to both adapters automatically.
//
// # Where to put the suite
//
// The suite lives next to the interface, in the `app` package, NOT in the
// adapter package. Reasons:
//
//   - it needs access to the interface and the sentinel errors;
//   - both adapter test files import it, so it must not depend on either
//     adapter;
//   - keeping it in `app` (rather than `app_test`) makes it importable
//     from `package adapter` test files without an import cycle.
//
// The function takes `*testing.T` from `testing` — the cost is a tiny
// import in production code; the win is one suite that runs against every
// adapter.
//
// # Conventions
//
//   - Each sub-test calls `newStorage()` once, so adapters that share state
//     between sub-tests can wipe it cleanly per case;
//   - postgres factories live behind `//go:build integration` and call a
//     local `wipeTables` helper before returning the adapter;
//   - sub-test names follow `Verb_What` (e.g. `Create_RoundTripsViaFind`),
//     so a single `go test -run` regex can target one clause across both
//     adapters;
//   - concurrency tests use `t.Parallel` + a barrier (`sync.WaitGroup`) to
//     prove atomicity rather than relying on race-detector luck.
//
// See `backend/promo/app/conformance.go` for the in-repo exemplar.
package storagetest
