package ratelimit_test

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/internal/ratelimit"
	_ "github.com/lib/pq"
	"github.com/matryer/is"
	"github.com/sirupsen/logrus"
)

func TestInMemoryLimiter_BurstThenDeny(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()

	rules := map[string]ratelimit.Rule{
		"test_action": {Rate: 1, Burst: 3},
	}
	limiter := ratelimit.NewInMemory(rules)

	is.True(limiter.Allow(ctx, "test_action", "ip-a"))  // 1/3
	is.True(limiter.Allow(ctx, "test_action", "ip-a"))  // 2/3
	is.True(limiter.Allow(ctx, "test_action", "ip-a"))  // 3/3
	is.True(!limiter.Allow(ctx, "test_action", "ip-a")) // bucket drained
}

func TestInMemoryLimiter_Isolation(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()

	rules := map[string]ratelimit.Rule{
		"action_a": {Rate: 1, Burst: 1},
		"action_b": {Rate: 1, Burst: 1},
	}
	limiter := ratelimit.NewInMemory(rules)

	// ip-1 uses action_a
	is.True(limiter.Allow(ctx, "action_a", "ip-1"))
	is.True(!limiter.Allow(ctx, "action_a", "ip-1"))

	// ip-2 uses action_a (isolated key)
	is.True(limiter.Allow(ctx, "action_a", "ip-2"))

	// ip-1 uses action_b (isolated action)
	is.True(limiter.Allow(ctx, "action_b", "ip-1"))
}

func TestPostgresLimiter_BurstThenDenyAndRefill(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/ecommerce?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil || db.Ping() != nil {
		t.Skip("PostgreSQL not available for integration test")
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DELETE FROM rate_limits WHERE key LIKE 'pg-test-%'")

	is := is.New(t)
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	testAction := "test_pg_action"
	rules := map[string]ratelimit.Rule{
		testAction: {Rate: 10, Burst: 2}, // 10 tokens/sec, burst 2
	}
	limiter := ratelimit.NewPostgres(db, rules, logger)

	key := "pg-test-client-1"

	// Initial burst
	is.True(limiter.Allow(ctx, testAction, key))  // 1/2
	is.True(limiter.Allow(ctx, testAction, key))  // 2/2
	is.True(!limiter.Allow(ctx, testAction, key)) // drained

	// Wait 250ms for refill (10 tokens/sec * 0.25s = 2.5 tokens -> refills back to burst 2)
	time.Sleep(250 * time.Millisecond)

	is.True(limiter.Allow(ctx, testAction, key))
}

func TestPostgresLimiter_ConcurrentAccess(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/ecommerce?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil || db.Ping() != nil {
		t.Skip("PostgreSQL not available for integration test")
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	key := "pg-test-concurrent"
	_, _ = db.ExecContext(ctx, "DELETE FROM rate_limits WHERE key = $1", key)

	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	burst := 5
	rules := map[string]ratelimit.Rule{
		"concurrent_action": {Rate: 0.001, Burst: burst}, // essentially no refill during test
	}
	limiter := ratelimit.NewPostgres(db, rules, logger)

	var wg sync.WaitGroup
	allowedCount := 0
	var mu sync.Mutex

	// Fire 20 concurrent goroutines representing multiple pods/requests hitting the limiter
	totalWorkers := 20
	wg.Add(totalWorkers)
	for i := 0; i < totalWorkers; i++ {
		go func() {
			defer wg.Done()
			if limiter.Allow(ctx, "concurrent_action", key) {
				mu.Lock()
				allowedCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if allowedCount != burst {
		t.Fatalf("expected exactly %d concurrent requests allowed, got %d", burst, allowedCount)
	}
}
