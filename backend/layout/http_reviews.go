package layout

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	reviewsApp "github.com/bkielbasa/go-ecommerce/backend/reviews/app"
	reviewsDomain "github.com/bkielbasa/go-ecommerce/backend/reviews/domain"
	"github.com/gorilla/mux"
)

// SubmitReview handles POST /product/{productID}/review. It requires a
// logged-in customer (anonymous browsers are bounced to the login page) and
// delegates the verified-buyer / duplicate-review checks to the reviews
// service, surfacing failure modes as friendly flash messages. On success
// the user is redirected back to the product page so the new review shows up
// in the list immediately.
func (handler httpHandler) SubmitReview(w http.ResponseWriter, r *http.Request) {
	customerID, ok := handler.requireLogin(w, r)
	if !ok {
		return
	}
	productID := mux.Vars(r)["productID"]
	if err := r.ParseForm(); err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	body := strings.TrimSpace(r.FormValue("body"))
	rating, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("rating")))

	err := handler.reviewsSrv.Submit(r.Context(), productID, customerID, body, rating)
	switch {
	case errors.Is(err, reviewsApp.ErrNotVerifiedBuyer):
		handler.flash(w, r, "Only verified buyers can leave a review.", "error")
	case errors.Is(err, reviewsApp.ErrDuplicateReview):
		handler.flash(w, r, "You have already reviewed this product.", "error")
	case errors.Is(err, reviewsDomain.ErrInvalidReview):
		handler.flash(w, r, err.Error(), "error")
	case err != nil:
		// Validation errors from domain.NewReview also flow here (they
		// don't wrap ErrInvalidReview today). Surfacing the message is
		// fine since it's deterministic ("rating must be between 1 and
		// 5" etc.).
		handler.flash(w, r, err.Error(), "error")
	default:
		handler.flash(w, r, "Thanks for your review!", "info")
	}
	http.Redirect(w, r, "/product/"+productID+"#reviews", http.StatusSeeOther)
}

// adminReviewRow is one row in the admin moderation table — a review plus
// the catalogue product it belongs to. The struct is exported (capitalised
// fields) so the html/template package can access its members.
type adminReviewRow struct {
	Review      reviewsDomain.Review
	ProductID   string
	ProductName string
}

// AdminReviews renders the moderation table. Newest first across all
// products; each row gets a small POST form that soft-deletes the review.
func (handler httpHandler) AdminReviews(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	// We do not have a global ListAll on the reviews service — that's an
	// admin-only need and the rest of the app only reads per-product. To
	// keep the service surface small we list per-product across the
	// catalogue. For small/medium stores this is fine; if it became hot
	// we'd add a dedicated query.
	products, err := handler.catalogSrv.AllProducts(r.Context())
	if err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	var rows []adminReviewRow
	for _, p := range products {
		list, err := handler.reviewsSrv.ListForProduct(r.Context(), string(p.ID()), 100)
		if err != nil {
			handler.logger.WithError(err).Warn("cannot list reviews for product")
			continue
		}
		for _, rv := range list {
			rows = append(rows, adminReviewRow{Review: rv, ProductID: string(p.ID()), ProductName: p.Name()})
		}
	}
	// Newest first across products.
	sortReviewRowsNewestFirst(rows)
	handler.renderAdminTemplate(w, r, "admin/reviews", map[string]any{
		"Active": "reviews",
		"Email":  email,
		"Rows":   rows,
	})
}

// AdminDeleteReview soft-deletes a review by id and bounces back to the
// moderation page. Admin-only; non-admins are rejected by requireAdmin.
func (handler httpHandler) AdminDeleteReview(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	if err := handler.reviewsSrv.Delete(r.Context(), id); err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	handler.flash(w, r, "Review removed.", "info")
	http.Redirect(w, r, "/admin/reviews", http.StatusSeeOther)
}

// sortReviewRowsNewestFirst sorts an admin review table in-place by
// CreatedAt descending. Pulled out of AdminReviews so the handler reads
// linearly.
func sortReviewRowsNewestFirst(rows []adminReviewRow) {
	// Simple insertion sort over a typically-small list keeps us off
	// sort.Slice (which would require a captured closure inside the
	// handler) and stays easy to read.
	for i := 1; i < len(rows); i++ {
		j := i
		for j > 0 && rows[j-1].Review.CreatedAt().Before(rows[j].Review.CreatedAt()) {
			rows[j-1], rows[j] = rows[j], rows[j-1]
			j--
		}
	}
}
