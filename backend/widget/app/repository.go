package app

import (
	"context"

	"github.com/bkielbasa/go-ecommerce/backend/widget/domain"
)

type WidgetRepository interface {
	Get(ctx context.Context, id string) (domain.Widget, error)
	ListAll(ctx context.Context) ([]domain.Widget, error)
	ListActiveByPages(ctx context.Context, pages ...domain.Page) ([]domain.Widget, error)
	Store(ctx context.Context, w domain.Widget) error
	Update(ctx context.Context, w domain.Widget) error
	Delete(ctx context.Context, id string) error

	StoreSlide(ctx context.Context, s domain.Slide) error
	DeleteSlide(ctx context.Context, widgetID, slideID string) error
}
