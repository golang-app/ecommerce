package adapter

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/search/app"
	"github.com/bkielbasa/go-ecommerce/backend/search/domain"
	"github.com/lib/pq"
)

// Postgres is the production Storage adapter. Documents live in a single
// search_document table with a stored, GIN-indexed tsvector aggregating
// title (weight A), body (weight B) and tags (weight C). All queries are
// parameterised; no user input is interpolated into the SQL string.
//
// Trade-off — text-search configuration: we use 'simple' rather than
// 'english' on purpose. 'english' applies stemming (e.g. "running" → "run")
// which is great for natural-language search but surprises in a demo where
// product names contain compounds and brand-ish nouns. 'simple' lowercases
// and tokenises only, matching the storefront's plain-substring mental
// model more closely. If the catalogue ever leans heavily on long-form
// copy, switching the config (and reindexing) is a one-migration change.
type Postgres struct {
	db *sql.DB
}

// NewPostgres wires the adapter against the shared *sql.DB.
func NewPostgres(db *sql.DB) *Postgres {
	return &Postgres{db: db}
}

// upsertSQL writes (or replaces) a document keyed by (kind, id). The
// generated `ts` column is automatically recomputed by postgres on every
// insert/update, so we never write to it explicitly.
const upsertSQL = `INSERT INTO search_document
		(kind, id, title, body, url, tags, meta, updated_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8)
	ON CONFLICT (kind, id) DO UPDATE SET
		title = EXCLUDED.title,
		body = EXCLUDED.body,
		url = EXCLUDED.url,
		tags = EXCLUDED.tags,
		meta = EXCLUDED.meta,
		updated_at = EXCLUDED.updated_at`

// Upsert serialises the document and writes it. meta is stored as jsonb;
// nil meta maps cleanly to "{}".
func (p *Postgres) Upsert(ctx context.Context, doc domain.Document) error {
	metaBytes, err := json.Marshal(doc.Meta())
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}
	updatedAt := doc.UpdatedAt()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	if _, err := p.db.ExecContext(ctx, upsertSQL,
		string(doc.Kind()),
		doc.ID(),
		doc.Title(),
		doc.Body(),
		doc.URL(),
		pq.Array(doc.Tags()),
		string(metaBytes),
		updatedAt,
	); err != nil {
		return fmt.Errorf("upsert search document: %w", err)
	}
	return nil
}

// Remove deletes the row keyed by (kind, id). Missing rows are a no-op.
func (p *Postgres) Remove(ctx context.Context, kind domain.Kind, id string) error {
	if _, err := p.db.ExecContext(ctx,
		`DELETE FROM search_document WHERE kind = $1 AND id = $2`,
		string(kind), id,
	); err != nil {
		return fmt.Errorf("delete search document: %w", err)
	}
	return nil
}

// querySQL runs a websearch_to_tsquery against the stored tsvector. The
// kind filter is optional: pass a NULL text[] (no Kinds option) to scan
// every kind. Ranking uses ts_rank_cd which prefers proximity-weighted
// matches over plain ts_rank.
const querySQL = `SELECT kind, id, title, body, url, tags, meta, updated_at,
		ts_rank_cd(ts, q) AS rank
	FROM search_document, websearch_to_tsquery('simple', $1) q
	WHERE ts @@ q
	  AND ($2::text[] IS NULL OR kind = ANY($2))
	ORDER BY rank DESC
	LIMIT $3`

// Query runs a full-text search. The kind filter is parameterised through
// pq.Array; a nil opts.Kinds passes NULL and matches every kind.
func (p *Postgres) Query(ctx context.Context, q string, opts app.QueryOptions) ([]app.Hit, error) {
	limit := opts.Limit
	if limit <= 0 {
		// Service applies its own default, but be defensive in case a
		// caller wires Storage directly (e.g. integration tests).
		limit = 50
	}

	var kindsArg interface{}
	if len(opts.Kinds) == 0 {
		kindsArg = nil
	} else {
		kinds := make([]string, 0, len(opts.Kinds))
		for _, k := range opts.Kinds {
			kinds = append(kinds, string(k))
		}
		kindsArg = pq.Array(kinds)
	}

	rows, err := p.db.QueryContext(ctx, querySQL, q, kindsArg, limit)
	if err != nil {
		return nil, fmt.Errorf("query search: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []app.Hit
	for rows.Next() {
		var (
			kind, id, title, body, url string
			tags                       []string
			metaRaw                    []byte
			updatedAt                  time.Time
			rank                       float64
		)
		if err := rows.Scan(&kind, &id, &title, &body, &url, pq.Array(&tags), &metaRaw, &updatedAt, &rank); err != nil {
			return nil, fmt.Errorf("scan search hit: %w", err)
		}
		meta := map[string]string{}
		if len(metaRaw) > 0 {
			// Tolerate a stray null jsonb (defensive — the column is
			// NOT NULL DEFAULT '{}' so this should never trigger).
			if err := json.Unmarshal(metaRaw, &meta); err != nil {
				return nil, fmt.Errorf("unmarshal meta: %w", err)
			}
		}
		out = append(out, app.Hit{
			Document: domain.RebuildDocument(domain.Kind(kind), id, title, body, url, tags, meta, updatedAt),
			Rank:     rank,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// RemoveAllOfKind clears every row of the given kind — used by the
// `reindex` CLI before walking the producer.
func (p *Postgres) RemoveAllOfKind(ctx context.Context, kind domain.Kind) error {
	if _, err := p.db.ExecContext(ctx,
		`DELETE FROM search_document WHERE kind = $1`,
		string(kind),
	); err != nil {
		return fmt.Errorf("remove kind: %w", err)
	}
	return nil
}
