package layout

import (
	"errors"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/gorilla/mux"

	repricingapp "github.com/bkielbasa/go-ecommerce/backend/repricing/app"
	repricingdomain "github.com/bkielbasa/go-ecommerce/backend/repricing/domain"
)

// AdminRepricing renders the bulk-reprice list page. It shows the
// currently-active reprice (if any), the past reprices table, and
// a form to start a new one. Admin-gated like every other /admin
// route.
func (handler httpHandler) AdminRepricing(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	if handler.repricingSrv == nil {
		handler.flash(w, r, "repricing service not wired", "error")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	rows, err := handler.repricingSrv.ListAll(r.Context())
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		rows = nil
	}
	active, hasActive, err := handler.repricingSrv.Active(r.Context())
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
	}
	categories, err := handler.catalogSrv.Categories(r.Context())
	if err != nil {
		handler.logger.WithError(err).Warn("cannot load categories for repricing form")
		categories = nil
	}
	handler.renderAdminTemplate(w, r, "admin/repricing", map[string]any{
		"Active":     "repricing",
		"Email":      email,
		"Reprices":   rows,
		"HasActive":  hasActive,
		"ActiveRow":  active,
		"Categories": categories,
	})
}

// AdminStartRepricing handles the start-reprice form submission. It
// reads the category slug + percent change, calls Service.Start and
// redirects to the detail page so the admin can watch the progress
// bar tick.
func (handler httpHandler) AdminStartRepricing(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	if handler.repricingSrv == nil {
		http.Redirect(w, r, "/admin/repricing", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	category := strings.TrimSpace(r.FormValue("category"))
	percent, perr := strconv.ParseFloat(strings.TrimSpace(r.FormValue("percent")), 64)
	if perr != nil {
		handler.flash(w, r, "Invalid percent value: "+perr.Error(), "error")
		http.Redirect(w, r, "/admin/repricing", http.StatusSeeOther)
		return
	}

	id, err := handler.repricingSrv.Start(r.Context(), category, percent)
	if err != nil {
		switch {
		case errors.Is(err, repricingapp.ErrAlreadyActive):
			handler.flash(w, r, "A reprice is already in progress; wait for it to finish.", "error")
		case errors.Is(err, repricingdomain.ErrInvalidReprice):
			handler.flash(w, r, "Invalid reprice: "+err.Error(), "error")
		default:
			handler.flash(w, r, err.Error(), "error")
		}
		http.Redirect(w, r, "/admin/repricing", http.StatusSeeOther)
		return
	}
	handler.flash(w, r, "Repricing started", "info")
	http.Redirect(w, r, "/admin/repricing/"+id, http.StatusSeeOther)
}

// AdminRepricingDetail renders the per-reprice detail page. On a
// normal GET it returns the full admin shell; when the request
// carries the HX-Request header (HTMX poll) it returns just the
// progress fragment so the page can refresh itself every couple of
// seconds without re-emitting the surrounding chrome.
func (handler httpHandler) AdminRepricingDetail(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	if handler.repricingSrv == nil {
		http.Redirect(w, r, "/admin/repricing", http.StatusSeeOther)
		return
	}
	id := mux.Vars(r)["id"]
	row, err := handler.repricingSrv.ByID(r.Context(), id)
	if errors.Is(err, repricingapp.ErrNotFound) {
		handler.flash(w, r, "Reprice not found", "error")
		http.Redirect(w, r, "/admin/repricing", http.StatusSeeOther)
		return
	}
	if err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}

	data := map[string]any{
		"Active":   "repricing",
		"Email":    email,
		"Reprice":  row,
		"Percent":  int(row.ProgressFraction() * 100),
		"Done":     row.Status() == repricingdomain.StatusCompleted || row.Status() == repricingdomain.StatusFailed,
		"InFlight": row.Status() == repricingdomain.StatusInProgress || row.Status() == repricingdomain.StatusScheduled,
	}

	if r.Header.Get("HX-Request") != "" {
		handler.renderRepricingProgressFragment(w, data)
		return
	}
	handler.renderAdminTemplate(w, r, "admin/repricing_detail", data)
}

// renderRepricingProgressFragment writes the progress block only,
// for HTMX polling. The fragment is parsed from the same template
// file as the full detail page (so a single source of truth defines
// both the embedded view and the polled fragment) but only the
// {{define "progress-block"}} chunk is executed.
func (handler httpHandler) renderRepricingProgressFragment(w http.ResponseWriter, data map[string]any) {
	ts, err := template.New("").ParseFiles("./layout/tmpl/admin/repricing_detail.gohtml")
	if err != nil {
		handler.logger.WithError(err).Error("cannot parse repricing fragment template")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if err := ts.ExecuteTemplate(w, "progress-block", data); err != nil {
		handler.logger.WithError(err).Error("cannot execute repricing progress fragment")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
