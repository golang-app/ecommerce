package app_test

import (
	"context"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/widget/app"
	"github.com/bkielbasa/go-ecommerce/backend/widget/domain"
)

type mockRepo struct {
	widgets map[string]domain.Widget
}

func newMockRepo() *mockRepo {
	return &mockRepo{widgets: make(map[string]domain.Widget)}
}

func (m *mockRepo) Get(ctx context.Context, id string) (domain.Widget, error) {
	w, ok := m.widgets[id]
	if !ok {
		return domain.Widget{}, domain.ErrWidgetNotFound
	}
	return w, nil
}

func (m *mockRepo) ListAll(ctx context.Context) ([]domain.Widget, error) {
	var list []domain.Widget
	for _, w := range m.widgets {
		list = append(list, w)
	}
	return list, nil
}

func (m *mockRepo) ListActiveByPages(ctx context.Context, pages ...domain.Page) ([]domain.Widget, error) {
	pageMap := make(map[domain.Page]bool)
	for _, p := range pages {
		pageMap[p] = true
	}
	var list []domain.Widget
	for _, w := range m.widgets {
		if w.IsActive && pageMap[w.Page] {
			list = append(list, w)
		}
	}
	return list, nil
}

func (m *mockRepo) Store(ctx context.Context, w domain.Widget) error {
	m.widgets[w.ID] = w
	return nil
}

func (m *mockRepo) Update(ctx context.Context, w domain.Widget) error {
	if _, ok := m.widgets[w.ID]; !ok {
		return domain.ErrWidgetNotFound
	}
	m.widgets[w.ID] = w
	return nil
}

func (m *mockRepo) Delete(ctx context.Context, id string) error {
	delete(m.widgets, id)
	return nil
}

func (m *mockRepo) StoreSlide(ctx context.Context, s domain.Slide) error {
	w, ok := m.widgets[s.WidgetID]
	if !ok {
		return domain.ErrWidgetNotFound
	}
	w.Slides = append(w.Slides, s)
	m.widgets[s.WidgetID] = w
	return nil
}

func (m *mockRepo) DeleteSlide(ctx context.Context, widgetID, slideID string) error {
	w, ok := m.widgets[widgetID]
	if !ok {
		return domain.ErrWidgetNotFound
	}
	var remaining []domain.Slide
	for _, s := range w.Slides {
		if s.ID != slideID {
			remaining = append(remaining, s)
		}
	}
	w.Slides = remaining
	m.widgets[widgetID] = w
	return nil
}

func TestService_ToggleActive(t *testing.T) {
	repo := newMockRepo()
	srv := app.NewService(repo)
	ctx := context.Background()

	w, _ := domain.NewWidget("w-1", domain.PageHome, domain.ZoneTop, domain.TypeBannerCarousel, "Banner", 1)
	w.IsActive = true
	_ = repo.Store(ctx, w)

	if err := srv.ToggleActive(ctx, "w-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated, _ := repo.Get(ctx, "w-1")
	if updated.IsActive {
		t.Errorf("expected widget to be inactive after toggle")
	}

	if err := srv.ToggleActive(ctx, "w-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reToggled, _ := repo.Get(ctx, "w-1")
	if !reToggled.IsActive {
		t.Errorf("expected widget to be active after second toggle")
	}
}
