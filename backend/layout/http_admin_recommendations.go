package layout

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/bkielbasa/go-ecommerce/backend/recommendation/domain"
)

// AdminRecommendations renders the recommendations admin page: the
// last refresh timestamp, the current weights (with the editable
// form), and the "refresh now" button.
//
// The handler is admin-gated like every other /admin route. When
// the recommendation service is not wired (composition root
// substituted a nil) the handler flashes an error and redirects
// back to /admin — same defensive pattern repricing uses.
func (handler httpHandler) AdminRecommendations(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	if handler.recommendationsSrv == nil {
		handler.flash(w, r, "recommendation service not wired", "error")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	weights, err := handler.recommendationsSrv.Weights(r.Context())
	if err != nil {
		handler.logger.WithError(err).Warn("cannot load recommendation weights")
		weights = domain.DefaultWeights()
	}
	lastRefresh, hasRefresh, err := handler.recommendationsSrv.LastRefresh(r.Context())
	if err != nil {
		handler.logger.WithError(err).Warn("cannot load recommendation last refresh")
	}
	handler.renderAdminTemplate(w, r, "admin/recommendations", map[string]any{
		"Active":      "recommendations",
		"Email":       email,
		"Weights":     weights,
		"LastRefresh": lastRefresh,
		"HasRefresh":  hasRefresh,
		"WeightsSum":  weights.Sum(),
	})
}

// AdminRefreshRecommendations kicks RefreshAll in a background
// goroutine and flashes a confirmation message. The goroutine runs
// on context.Background() — same trade-off the repricing saga uses
// — so cancelling the admin's HTTP request does not abort the
// refresh. The handler returns immediately (303 See Other) so the
// admin's browser does not hang while the pass runs.
func (handler httpHandler) AdminRefreshRecommendations(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	if handler.recommendationsRfr == nil {
		handler.flash(w, r, "recommendation refresher not wired", "error")
		http.Redirect(w, r, "/admin/recommendations", http.StatusSeeOther)
		return
	}
	go func() {
		if err := handler.recommendationsRfr.RefreshAll(context.Background()); err != nil {
			handler.logger.WithError(err).Warn("manual recommendation refresh failed")
		}
	}()
	handler.flash(w, r, "Recommendation refresh started in the background.", "info")
	http.Redirect(w, r, "/admin/recommendations", http.StatusSeeOther)
}

// AdminSaveRecommendationWeights parses the five form inputs,
// validates non-negative via the domain constructor, persists and
// redirects back with a flash. Invalid inputs (parse failure or
// negative weight) re-render with an error flash; on success the
// next refresher tick picks up the new values automatically.
func (handler httpHandler) AdminSaveRecommendationWeights(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	if handler.recommendationsSrv == nil {
		http.Redirect(w, r, "/admin/recommendations", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	parse := func(name string) (float64, error) {
		raw := strings.TrimSpace(r.FormValue(name))
		if raw == "" {
			return 0, nil
		}
		return strconv.ParseFloat(raw, 64)
	}
	co, err := parse("weight_copurchase")
	if err != nil {
		handler.flash(w, r, "Invalid co-purchase weight: "+err.Error(), "error")
		http.Redirect(w, r, "/admin/recommendations", http.StatusSeeOther)
		return
	}
	txt, err := parse("weight_text")
	if err != nil {
		handler.flash(w, r, "Invalid text weight: "+err.Error(), "error")
		http.Redirect(w, r, "/admin/recommendations", http.StatusSeeOther)
		return
	}
	cat, err := parse("weight_category")
	if err != nil {
		handler.flash(w, r, "Invalid category weight: "+err.Error(), "error")
		http.Redirect(w, r, "/admin/recommendations", http.StatusSeeOther)
		return
	}
	attr, err := parse("weight_attributes")
	if err != nil {
		handler.flash(w, r, "Invalid attributes weight: "+err.Error(), "error")
		http.Redirect(w, r, "/admin/recommendations", http.StatusSeeOther)
		return
	}
	price, err := parse("weight_price")
	if err != nil {
		handler.flash(w, r, "Invalid price weight: "+err.Error(), "error")
		http.Redirect(w, r, "/admin/recommendations", http.StatusSeeOther)
		return
	}

	w0 := domain.Weights{
		CoPurchase: co,
		Text:       txt,
		Category:   cat,
		Attributes: attr,
		Price:      price,
	}
	if err := handler.recommendationsSrv.SetWeights(r.Context(), w0); err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidWeights):
			handler.flash(w, r, "Weights must be non-negative.", "error")
		default:
			handler.flash(w, r, "Failed to save weights: "+err.Error(), "error")
		}
		http.Redirect(w, r, "/admin/recommendations", http.StatusSeeOther)
		return
	}
	handler.flash(w, r, "Recommendation weights saved.", "info")
	http.Redirect(w, r, "/admin/recommendations", http.StatusSeeOther)
}
