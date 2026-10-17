package adapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/bkielbasa/go-ecommerce/backend/widget/domain"
	"github.com/lib/pq"
)

type Postgres struct {
	db *sql.DB
}

func NewPostgres(db *sql.DB) *Postgres {
	return &Postgres{db: db}
}

func (p *Postgres) Get(ctx context.Context, id string) (domain.Widget, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT id, page, zone, type, title, config, display_order, is_active, created_at, updated_at
		FROM widget
		WHERE id = $1
	`, id)

	var w domain.Widget
	var page, zone, widgetType string
	if err := row.Scan(&w.ID, &page, &zone, &widgetType, &w.Title, &w.Config, &w.DisplayOrder, &w.IsActive, &w.CreatedAt, &w.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Widget{}, domain.ErrWidgetNotFound
		}
		return domain.Widget{}, fmt.Errorf("failed to scan widget: %w", err)
	}
	w.Page = domain.Page(page)
	w.Zone = domain.Zone(zone)
	w.Type = domain.Type(widgetType)

	slides, err := p.loadSlides(ctx, []string{w.ID})
	if err != nil {
		return domain.Widget{}, err
	}
	w.Slides = slides[w.ID]
	if w.Slides == nil {
		w.Slides = []domain.Slide{}
	}
	return w, nil
}

func (p *Postgres) ListAll(ctx context.Context) ([]domain.Widget, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, page, zone, type, title, config, display_order, is_active, created_at, updated_at
		FROM widget
		ORDER BY page ASC, zone ASC, display_order ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list widgets: %w", err)
	}
	defer rows.Close()

	var widgets []domain.Widget
	var ids []string
	for rows.Next() {
		var w domain.Widget
		var page, zone, widgetType string
		if err := rows.Scan(&w.ID, &page, &zone, &widgetType, &w.Title, &w.Config, &w.DisplayOrder, &w.IsActive, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan widget row: %w", err)
		}
		w.Page = domain.Page(page)
		w.Zone = domain.Zone(zone)
		w.Type = domain.Type(widgetType)
		widgets = append(widgets, w)
		ids = append(ids, w.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	slides, err := p.loadSlides(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range widgets {
		widgets[i].Slides = slides[widgets[i].ID]
		if widgets[i].Slides == nil {
			widgets[i].Slides = []domain.Slide{}
		}
	}
	return widgets, nil
}

func (p *Postgres) ListActiveByPages(ctx context.Context, pages ...domain.Page) ([]domain.Widget, error) {
	pageStrings := make([]string, len(pages))
	for i, pg := range pages {
		pageStrings[i] = string(pg)
	}

	rows, err := p.db.QueryContext(ctx, `
		SELECT id, page, zone, type, title, config, display_order, is_active, created_at, updated_at
		FROM widget
		WHERE is_active = true AND page = ANY($1)
		ORDER BY zone ASC, display_order ASC
	`, pq.Array(pageStrings))
	if err != nil {
		return nil, fmt.Errorf("failed to list active widgets: %w", err)
	}
	defer rows.Close()

	var widgets []domain.Widget
	var ids []string
	for rows.Next() {
		var w domain.Widget
		var page, zone, widgetType string
		if err := rows.Scan(&w.ID, &page, &zone, &widgetType, &w.Title, &w.Config, &w.DisplayOrder, &w.IsActive, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan active widget: %w", err)
		}
		w.Page = domain.Page(page)
		w.Zone = domain.Zone(zone)
		w.Type = domain.Type(widgetType)
		widgets = append(widgets, w)
		ids = append(ids, w.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	slides, err := p.loadSlides(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range widgets {
		widgets[i].Slides = slides[widgets[i].ID]
		if widgets[i].Slides == nil {
			widgets[i].Slides = []domain.Slide{}
		}
	}
	return widgets, nil
}

func (p *Postgres) Store(ctx context.Context, w domain.Widget) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO widget (id, page, zone, type, title, config, display_order, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, w.ID, string(w.Page), string(w.Zone), string(w.Type), w.Title, []byte(w.Config), w.DisplayOrder, w.IsActive, w.CreatedAt, w.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert widget: %w", err)
	}
	return nil
}

func (p *Postgres) Update(ctx context.Context, w domain.Widget) error {
	res, err := p.db.ExecContext(ctx, `
		UPDATE widget
		SET page = $2, zone = $3, type = $4, title = $5, config = $6, display_order = $7, is_active = $8, updated_at = $9
		WHERE id = $1
	`, w.ID, string(w.Page), string(w.Zone), string(w.Type), w.Title, []byte(w.Config), w.DisplayOrder, w.IsActive, w.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to update widget: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrWidgetNotFound
	}
	return nil
}

func (p *Postgres) Delete(ctx context.Context, id string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM widget WHERE id = $1`, id)
	return err
}

func (p *Postgres) StoreSlide(ctx context.Context, s domain.Slide) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO widget_slide (id, widget_id, title, subtitle, link_url, link_text, promo_code, badge, image_url, display_order, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			title = EXCLUDED.title,
			subtitle = EXCLUDED.subtitle,
			link_url = EXCLUDED.link_url,
			link_text = EXCLUDED.link_text,
			promo_code = EXCLUDED.promo_code,
			badge = EXCLUDED.badge,
			image_url = EXCLUDED.image_url,
			display_order = EXCLUDED.display_order,
			updated_at = EXCLUDED.updated_at
	`, s.ID, s.WidgetID, s.Title, s.Subtitle, s.LinkURL, s.LinkText, s.PromoCode, s.Badge, s.ImageURL, s.DisplayOrder, s.CreatedAt, s.UpdatedAt)
	return err
}

func (p *Postgres) DeleteSlide(ctx context.Context, widgetID, slideID string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM widget_slide WHERE widget_id = $1 AND id = $2`, widgetID, slideID)
	return err
}

func (p *Postgres) loadSlides(ctx context.Context, widgetIDs []string) (map[string][]domain.Slide, error) {
	result := make(map[string][]domain.Slide)
	if len(widgetIDs) == 0 {
		return result, nil
	}

	rows, err := p.db.QueryContext(ctx, `
		SELECT id, widget_id, title, subtitle, link_url, link_text, promo_code, badge, image_url, display_order, created_at, updated_at
		FROM widget_slide
		WHERE widget_id = ANY($1)
		ORDER BY display_order ASC
	`, pq.Array(widgetIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to load slides: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var s domain.Slide
		if err := rows.Scan(&s.ID, &s.WidgetID, &s.Title, &s.Subtitle, &s.LinkURL, &s.LinkText, &s.PromoCode, &s.Badge, &s.ImageURL, &s.DisplayOrder, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan slide: %w", err)
		}
		result[s.WidgetID] = append(result[s.WidgetID], s)
	}
	return result, rows.Err()
}
