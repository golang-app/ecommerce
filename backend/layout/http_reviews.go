package layout

import (
	"context"
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
		// Reviews now go through a moderation queue; the storefront list
		// is filtered to approved-only, so a successful Submit is invisible
		// to the customer until an admin approves it. The flash explains
		// the new flow so the silent product-page redirect doesn't feel
		// broken.
		handler.flash(w, r, "Thanks — your review is pending moderation and will appear once approved.", "info")
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

// AdminReviews renders the moderation table. The default tab is "pending"
// (the queue of submissions awaiting an admin decision); the "all" tab
// shows every non-deleted review regardless of status. Each row gets per-
// status action buttons (approve / reject / delete) so an admin can make
// the next decision without leaving the page.
func (handler httpHandler) AdminReviews(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}

	// Default tab is "pending" — the moderation queue. "all" shows every
	// review (any status) so an admin can also reverse an earlier decision.
	statusParam := strings.TrimSpace(r.URL.Query().Get("status"))
	if statusParam != "all" {
		statusParam = "pending"
	}

	var (
		reviews []reviewsDomain.Review
		err     error
	)
	if statusParam == "all" {
		reviews, err = handler.reviewsSrv.ListAll(r.Context(), 200)
	} else {
		reviews, err = handler.reviewsSrv.ListPending(r.Context(), 200)
	}
	if err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}

	// The pending count drives the tab label even on the "all" tab so the
	// admin sees the queue size at a glance.
	pendingCount, err := handler.pendingReviewCount(r.Context())
	if err != nil {
		handler.logger.WithError(err).Warn("cannot count pending reviews")
	}

	// Build the row decoration (product id + name) by resolving each
	// review's product. We cache lookups so a product with many reviews
	// only hits the catalogue once.
	rows := handler.decorateReviewRows(r, reviews)

	handler.renderAdminTemplate(w, r, "admin/reviews", map[string]any{
		"Active":       "reviews",
		"Email":        email,
		"Rows":         rows,
		"ActiveTab":    statusParam,
		"PendingCount": pendingCount,
	})
}

// pendingReviewCount returns the total pending-review backlog so the tab
// label can show "Pending (N)". A non-blocking helper — callers log and
// continue when this fails.
func (handler httpHandler) pendingReviewCount(ctx context.Context) (int, error) {
	pending, err := handler.reviewsSrv.ListPending(ctx, 1000)
	if err != nil {
		return 0, err
	}
	return len(pending), nil
}

// decorateReviewRows attaches the product id + display name to each review.
// We resolve each unique product once (the catalogue lookup is cheap but
// repeating it per review row would be wasteful when a product accumulates
// many reviews).
func (handler httpHandler) decorateReviewRows(r *http.Request, reviews []reviewsDomain.Review) []adminReviewRow {
	type productInfo struct {
		name string
		ok   bool
	}
	cache := map[string]productInfo{}
	rows := make([]adminReviewRow, 0, len(reviews))
	for _, rv := range reviews {
		pid := rv.ProductID()
		info, seen := cache[pid]
		if !seen {
			p, err := handler.catalogSrv.Find(r.Context(), pid)
			if err != nil {
				handler.logger.WithError(err).Warn("cannot find product for review")
				info = productInfo{name: pid, ok: false}
			} else {
				info = productInfo{name: p.Name(), ok: true}
			}
			cache[pid] = info
		}
		rows = append(rows, adminReviewRow{Review: rv, ProductID: pid, ProductName: info.name})
	}
	return rows
}

// AdminApproveReview flips a review to approved status and bounces back to
// the moderation page (preserving the active tab via ?status=).
func (handler httpHandler) AdminApproveReview(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	if err := handler.reviewsSrv.Approve(r.Context(), id); err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	handler.flash(w, r, "Review approved.", "info")
	http.Redirect(w, r, adminReviewsRedirect(r), http.StatusSeeOther)
}

// AdminRejectReview flips a review to rejected status and bounces back to
// the moderation page (preserving the active tab via ?status=).
func (handler httpHandler) AdminRejectReview(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	if err := handler.reviewsSrv.Reject(r.Context(), id); err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	handler.flash(w, r, "Review rejected.", "info")
	http.Redirect(w, r, adminReviewsRedirect(r), http.StatusSeeOther)
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
	http.Redirect(w, r, adminReviewsRedirect(r), http.StatusSeeOther)
}

// adminReviewsRedirect builds the redirect target after a moderation
// action so the admin stays on the same tab. The status query param is
// passed in via the form (hidden input) so we honour it even though POST
// requests don't carry the prior page's query string.
func adminReviewsRedirect(r *http.Request) string {
	status := strings.TrimSpace(r.FormValue("status"))
	if status == "all" {
		return "/admin/reviews?status=all"
	}
	return "/admin/reviews"
}
