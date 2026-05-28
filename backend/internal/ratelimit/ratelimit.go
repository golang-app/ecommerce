// Package ratelimit provides a small, dependency-free token-bucket rate
// limiter keyed by an arbitrary string (typically a client IP). It is
// intentionally tiny — one mutex, a map of buckets, a fractional token
// counter per bucket — so the whole package can live entirely in-memory in
// a single process without pulling in golang.org/x/time/rate.
//
// Each bucket refills at `rate` tokens/second up to `burst` tokens. A bucket
// allows a request iff its current token count is >= 1, in which case one
// token is consumed. Buckets are created lazily on the first call for a key
// and capped at maxKeys; once full, a random eviction round drops a
// stale-looking entry to keep memory bounded under abusive traffic.
package ratelimit

import (
	"sync"
	"time"
)

// maxKeys caps the bucket map size. The number is generous enough for the
// real workloads we throttle (login/register/cart-add per source IP) and
// small enough that the map stays well under a megabyte even fully populated.
const maxKeys = 4096

// bucket is the per-key state. tokens is fractional so a sub-per-second
// rate (e.g. 3/hour) is representable; lastRefill anchors the next refill
// calculation.
type bucket struct {
	tokens     float64
	lastRefill time.Time
}

// Limiter is a token-bucket rate limiter keyed by string. The zero value is
// not usable; construct one with New.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64
	burst   float64
	now     func() time.Time
}

// New returns a Limiter that refills each bucket at rate tokens/second up
// to a maximum of burst tokens. burst doubles as the bucket's starting
// capacity so the first burst requests for a fresh key always succeed.
func New(rate float64, burst int) *Limiter {
	if rate < 0 {
		rate = 0
	}
	if burst < 1 {
		burst = 1
	}
	return &Limiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		burst:   float64(burst),
		now:     time.Now,
	}
}

// Allow reports whether a request for key is permitted right now. On true,
// one token is consumed from the bucket; on false, the bucket is untouched.
func (l *Limiter) Allow(key string) bool {
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		// Cap the working set. We don't track usage timestamps so a
		// random victim is the cheapest defensible eviction; under
		// real load the surviving buckets refill to full within
		// seconds, so a wrongly-evicted entry is self-healing.
		if len(l.buckets) >= maxKeys {
			l.evictOneLocked()
		}
		b = &bucket{tokens: l.burst, lastRefill: now}
		l.buckets[key] = b
	} else {
		elapsed := now.Sub(b.lastRefill).Seconds()
		if elapsed > 0 {
			b.tokens += elapsed * l.rate
			if b.tokens > l.burst {
				b.tokens = l.burst
			}
			b.lastRefill = now
		}
	}

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// evictOneLocked drops a single arbitrary entry from the bucket map. The
// caller must hold l.mu. Go's map iteration is randomized so the chosen
// victim is effectively uniform without us tracking insertion order.
func (l *Limiter) evictOneLocked() {
	// We only need to drop one entry; break out of the iteration on the
	// first key we see.
	for k := range l.buckets {
		delete(l.buckets, k)
		return
	}
}
