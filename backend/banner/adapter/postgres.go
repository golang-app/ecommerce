package adapter

import (
	"context"
	"database/sql"
	"errors"

	"github.com/bkielbasa/go-ecommerce/backend/banner/domain"
)

type Postgres struct {
	db *sql.DB
}

func NewPostgres(db *sql.DB) *Postgres {
	return &Postgres{db: db}
}

func (p *Postgres) Get(ctx context.Context, id string) (domain.Banner, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT id, slot, title, body, link_url, link_text, promo_code, icon, image_url, display_order, is_active, created_at, updated_at
		FROM banner WHERE id = $1`, id)
	return scanBanner(row)
}

func (p *Postgres) ListAll(ctx context.Context) ([]domain.Banner, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, slot, title, body, link_url, link_text, promo_code, icon, image_url, display_order, is_active, created_at, updated_at
		FROM banner ORDER BY slot, display_order ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBanners(rows)
}

func (p *Postgres) ListActiveBySlot(ctx context.Context, slot domain.Slot) ([]domain.Banner, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, slot, title, body, link_url, link_text, promo_code, icon, image_url, display_order, is_active, created_at, updated_at
		FROM banner WHERE slot = $1 AND is_active = TRUE ORDER BY display_order ASC`, string(slot))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBanners(rows)
}

func (p *Postgres) Store(ctx context.Context, b domain.Banner) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO banner (id, slot, title, body, link_url, link_text, promo_code, icon, image_url, display_order, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		b.ID, string(b.Slot), b.Title, b.Body, b.LinkURL, b.LinkText, b.PromoCode, b.Icon, b.ImageURL, b.DisplayOrder, b.IsActive, b.CreatedAt, b.UpdatedAt)
	return err
}

func (p *Postgres) Update(ctx context.Context, b domain.Banner) error {
	res, err := p.db.ExecContext(ctx, `
		UPDATE banner SET slot = $2, title = $3, body = $4, link_url = $5, link_text = $6, promo_code = $7, icon = $8, image_url = $9, display_order = $10, is_active = $11, updated_at = $12
		WHERE id = $1`,
		b.ID, string(b.Slot), b.Title, b.Body, b.LinkURL, b.LinkText, b.PromoCode, b.Icon, b.ImageURL, b.DisplayOrder, b.IsActive, b.UpdatedAt)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrBannerNotFound
	}
	return nil
}

func (p *Postgres) Delete(ctx context.Context, id string) error {
	res, err := p.db.ExecContext(ctx, `DELETE FROM banner WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrBannerNotFound
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanBanner(s scanner) (domain.Banner, error) {
	var b domain.Banner
	var slotStr string
	err := s.Scan(&b.ID, &slotStr, &b.Title, &b.Body, &b.LinkURL, &b.LinkText, &b.PromoCode, &b.Icon, &b.ImageURL, &b.DisplayOrder, &b.IsActive, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Banner{}, domain.ErrBannerNotFound
		}
		return domain.Banner{}, err
	}
	b.Slot = domain.Slot(slotStr)
	return b, nil
}

func scanBanners(rows *sql.Rows) ([]domain.Banner, error) {
	var out []domain.Banner
	for rows.Next() {
		b, err := scanBanner(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
