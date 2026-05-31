package adapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/recommendation/domain"
)

// Postgres backs the Storage port with the recommendation_link
// table. UpsertTopN runs DELETE-then-INSERT inside one transaction
// so a concurrent storefront read either sees the previous list in
// full or the next one in full — never a half-built mix.
type Postgres struct {
	db *sql.DB
}

// NewPostgres wires the storage adapter against the shared DB.
func NewPostgres(db *sql.DB) *Postgres {
	return &Postgres{db: db}
}

// UpsertTopN replaces the seed's previous top-N list with `recs`.
// The transaction wraps a DELETE + N INSERTs so the storefront's
// SELECT either reads the pre-tx state or the post-tx state.
// An empty `recs` slice still runs the DELETE so a refresher that
// has decided "this seed has nothing to recommend" can clear stale
// rows from a previous run.
func (p *Postgres) UpsertTopN(ctx context.Context, productID string, recs []domain.Recommendation) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM recommendation_link WHERE product_id = $1`, productID); err != nil {
		return fmt.Errorf("delete old recs: %w", err)
	}
	now := time.Now().UTC()
	for _, r := range recs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO recommendation_link
			    (product_id, recommended_id, score, position, computed_at)
			VALUES ($1, $2, $3, $4, $5)
		`, r.ProductID, r.RecommendedID, float64(r.Score), r.Position, now); err != nil {
			return fmt.Errorf("insert rec: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	committed = true
	return nil
}

// TopN reads up to `limit` rows for the seed, ordered by position.
func (p *Postgres) TopN(ctx context.Context, productID string, limit int) ([]domain.Recommendation, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := p.db.QueryContext(ctx, `
		SELECT product_id, recommended_id, score, position
		FROM recommendation_link
		WHERE product_id = $1
		ORDER BY position ASC
		LIMIT $2
	`, productID, limit)
	if err != nil {
		return nil, fmt.Errorf("query top-n: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Recommendation
	for rows.Next() {
		var (
			pid, rid string
			score    float64
			pos      int
		)
		if err := rows.Scan(&pid, &rid, &score, &pos); err != nil {
			return nil, fmt.Errorf("scan rec: %w", err)
		}
		out = append(out, domain.Recommendation{
			ProductID:     pid,
			RecommendedID: rid,
			Score:         domain.Score(score),
			Position:      pos,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// LastComputedAt returns the most recent computed_at for the seed.
func (p *Postgres) LastComputedAt(ctx context.Context, productID string) (time.Time, bool, error) {
	var t time.Time
	err := p.db.QueryRowContext(ctx, `
		SELECT MAX(computed_at)
		FROM recommendation_link
		WHERE product_id = $1
	`, productID).Scan(&t)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		// MAX on no rows returns NULL, which Scan into time.Time
		// surfaces as an unsupported-conversion error. Treat any
		// NULL-shaped failure as "no row" so the caller's
		// "never refreshed" branch fires.
		var nt sql.NullTime
		retry := p.db.QueryRowContext(ctx, `
			SELECT MAX(computed_at)
			FROM recommendation_link
			WHERE product_id = $1
		`, productID).Scan(&nt)
		if retry != nil {
			return time.Time{}, false, fmt.Errorf("last computed: %w", retry)
		}
		if !nt.Valid {
			return time.Time{}, false, nil
		}
		return nt.Time, true, nil
	}
	return t, true, nil
}

// OverallLastRefresh returns the latest computed_at across the
// whole table — used by the admin dashboard's "last refreshed at"
// line. ok=false when the table is empty.
func (p *Postgres) OverallLastRefresh(ctx context.Context) (time.Time, bool, error) {
	var nt sql.NullTime
	err := p.db.QueryRowContext(ctx, `SELECT MAX(computed_at) FROM recommendation_link`).Scan(&nt)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("overall last refresh: %w", err)
	}
	if !nt.Valid {
		return time.Time{}, false, nil
	}
	return nt.Time, true, nil
}

// PostgresWeights backs the WeightsStorage port against the
// singleton recommendation_weights row (id=1). The migration seeds
// the row so Get should always find it; a missing row falls back to
// domain.DefaultWeights() so the demo keeps running.
type PostgresWeights struct {
	db *sql.DB
}

// NewPostgresWeights wires the weights adapter.
func NewPostgresWeights(db *sql.DB) *PostgresWeights {
	return &PostgresWeights{db: db}
}

// Get returns the persisted singleton row, or defaults when the row
// is missing.
func (p *PostgresWeights) Get(ctx context.Context) (domain.Weights, error) {
	var co, txt, cat, attr, price float64
	err := p.db.QueryRowContext(ctx, `
		SELECT weight_copurchase, weight_text, weight_category, weight_attributes, weight_price
		FROM recommendation_weights
		WHERE id = 1
	`).Scan(&co, &txt, &cat, &attr, &price)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DefaultWeights(), nil
	}
	if err != nil {
		return domain.Weights{}, fmt.Errorf("load weights: %w", err)
	}
	return domain.Weights{
		CoPurchase: co,
		Text:       txt,
		Category:   cat,
		Attributes: attr,
		Price:      price,
	}, nil
}

// Set overwrites the singleton row and bumps updated_at. An
// upsert keeps the admin write path robust even if the seed INSERT
// in migration 000041 was somehow rolled back.
func (p *PostgresWeights) Set(ctx context.Context, w domain.Weights) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO recommendation_weights (id, weight_copurchase, weight_text, weight_category, weight_attributes, weight_price, updated_at)
		VALUES (1, $1, $2, $3, $4, $5, now())
		ON CONFLICT (id) DO UPDATE SET
		    weight_copurchase = EXCLUDED.weight_copurchase,
		    weight_text = EXCLUDED.weight_text,
		    weight_category = EXCLUDED.weight_category,
		    weight_attributes = EXCLUDED.weight_attributes,
		    weight_price = EXCLUDED.weight_price,
		    updated_at = EXCLUDED.updated_at
	`, w.CoPurchase, w.Text, w.Category, w.Attributes, w.Price)
	if err != nil {
		return fmt.Errorf("save weights: %w", err)
	}
	return nil
}
