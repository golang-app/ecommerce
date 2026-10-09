package adapter

import (
	"context"
	"sort"
	"sync"

	"github.com/bkielbasa/go-ecommerce/backend/widget/domain"
)

type InMemory struct {
	mu      sync.RWMutex
	widgets map[string]domain.Widget
	slides  map[string][]domain.Slide
}

func NewInMemory() *InMemory {
	return &InMemory{
		widgets: make(map[string]domain.Widget),
		slides:  make(map[string][]domain.Slide),
	}
}

func (m *InMemory) Get(ctx context.Context, id string) (domain.Widget, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	w, ok := m.widgets[id]
	if !ok {
		return domain.Widget{}, domain.ErrWidgetNotFound
	}
	w.Slides = m.copySlides(m.slides[id])
	return w, nil
}

func (m *InMemory) ListAll(ctx context.Context) ([]domain.Widget, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []domain.Widget
	for _, w := range m.widgets {
		w.Slides = m.copySlides(m.slides[w.ID])
		result = append(result, w)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Page != result[j].Page {
			return result[i].Page < result[j].Page
		}
		if result[i].Zone != result[j].Zone {
			return result[i].Zone < result[j].Zone
		}
		return result[i].DisplayOrder < result[j].DisplayOrder
	})
	return result, nil
}

func (m *InMemory) ListActiveByPages(ctx context.Context, pages ...domain.Page) ([]domain.Widget, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pageMap := make(map[domain.Page]bool)
	for _, p := range pages {
		pageMap[p] = true
	}

	var result []domain.Widget
	for _, w := range m.widgets {
		if w.IsActive && pageMap[w.Page] {
			w.Slides = m.copySlides(m.slides[w.ID])
			result = append(result, w)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Zone != result[j].Zone {
			return result[i].Zone < result[j].Zone
		}
		return result[i].DisplayOrder < result[j].DisplayOrder
	})
	return result, nil
}

func (m *InMemory) Store(ctx context.Context, w domain.Widget) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.widgets[w.ID] = w
	return nil
}

func (m *InMemory) Update(ctx context.Context, w domain.Widget) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.widgets[w.ID]; !ok {
		return domain.ErrWidgetNotFound
	}
	m.widgets[w.ID] = w
	return nil
}

func (m *InMemory) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.widgets, id)
	delete(m.slides, id)
	return nil
}

func (m *InMemory) StoreSlide(ctx context.Context, s domain.Slide) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.widgets[s.WidgetID]; !ok {
		return domain.ErrWidgetNotFound
	}
	slides := m.slides[s.WidgetID]
	updated := false
	for i, existing := range slides {
		if existing.ID == s.ID {
			slides[i] = s
			updated = true
			break
		}
	}
	if !updated {
		slides = append(slides, s)
	}
	sort.Slice(slides, func(i, j int) bool {
		return slides[i].DisplayOrder < slides[j].DisplayOrder
	})
	m.slides[s.WidgetID] = slides
	return nil
}

func (m *InMemory) DeleteSlide(ctx context.Context, widgetID, slideID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	slides := m.slides[widgetID]
	var remaining []domain.Slide
	for _, s := range slides {
		if s.ID != slideID {
			remaining = append(remaining, s)
		}
	}
	m.slides[widgetID] = remaining
	return nil
}

func (m *InMemory) copySlides(slides []domain.Slide) []domain.Slide {
	if len(slides) == 0 {
		return []domain.Slide{}
	}
	cp := make([]domain.Slide, len(slides))
	copy(cp, slides)
	return cp
}
