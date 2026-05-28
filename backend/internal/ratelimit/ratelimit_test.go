package ratelimit

import (
	"testing"
	"time"

	"github.com/matryer/is"
)

// TestAllowBurstThenDeny exercises the documented contract: a fresh key is
// allowed up to `burst` times in a tight loop with no refill, then refused.
func TestAllowBurstThenDeny(t *testing.T) {
	is := is.New(t)

	// 1 token per second, burst of 3 — using a frozen clock so refill is
	// strictly opt-in via the helper below.
	l := New(1, 3)
	frozen := time.Unix(1_700_000_000, 0)
	l.now = func() time.Time { return frozen }

	is.True(l.Allow("ip-a"))  // 1/3
	is.True(l.Allow("ip-a"))  // 2/3
	is.True(l.Allow("ip-a"))  // 3/3
	is.True(!l.Allow("ip-a")) // bucket drained, must refuse
}

// TestAllowRefillsAfterTime confirms that after enough wall-clock has elapsed
// the bucket refills and Allow returns true again.
func TestAllowRefillsAfterTime(t *testing.T) {
	is := is.New(t)

	// 5 tokens per second, burst of 2 — drain, then advance 1 s which
	// must add 5 tokens (capped to burst=2).
	l := New(5, 2)
	frozen := time.Unix(1_700_000_000, 0)
	l.now = func() time.Time { return frozen }

	is.True(l.Allow("ip-b"))  // 1/2
	is.True(l.Allow("ip-b"))  // 2/2
	is.True(!l.Allow("ip-b")) // drained

	// Advance one second; bucket should be back to full.
	frozen = frozen.Add(time.Second)
	is.True(l.Allow("ip-b"))
	is.True(l.Allow("ip-b"))
	is.True(!l.Allow("ip-b"))
}

// TestPerKeyIsolation makes sure one noisy key does not poison another's
// bucket.
func TestPerKeyIsolation(t *testing.T) {
	is := is.New(t)

	l := New(1, 2)
	frozen := time.Unix(1_700_000_000, 0)
	l.now = func() time.Time { return frozen }

	is.True(l.Allow("noisy"))
	is.True(l.Allow("noisy"))
	is.True(!l.Allow("noisy"))

	// A different key has its own untouched bucket.
	is.True(l.Allow("quiet"))
	is.True(l.Allow("quiet"))
	is.True(!l.Allow("quiet"))
}
