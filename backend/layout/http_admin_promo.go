package layout

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	promoapp "github.com/bkielbasa/go-ecommerce/backend/promo/app"
	promodomain "github.com/bkielbasa/go-ecommerce/backend/promo/domain"
	"github.com/gorilla/mux"
)

// AdminPromoCodes renders the promo-codes list page with the inline
// "create" form. The handler is admin-gated like every other /admin route.
func (handler httpHandler) AdminPromoCodes(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	codes, err := handler.promoSrv.ListAll(r.Context())
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		codes = nil
	}
	handler.renderAdminTemplate(w, r, "admin/promo_codes", map[string]any{
		"Active": "promo-codes",
		"Email":  email,
		"Codes":  codes,
	})
}

// AdminCreatePromoCode handles the create-promo-code form submission.
//
// Idempotency. This endpoint participates in the HTTP-boundary
// Idempotency-Key contract (see internal/idempotency): a client may
// send the same `Idempotency-Key` header on a retry and the
// originally-recorded response will be replayed instead of attempting
// to create a duplicate promo code (which the domain would reject with
// ErrCodeAlreadyExists anyway — the contract just makes the retry
// path return the original "created" response cleanly).
func (handler httpHandler) AdminCreatePromoCode(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	c, err := promoCodeFromForm(r, r.FormValue("code"))
	if err != nil {
		handler.flash(w, r, "Invalid promo code: "+err.Error(), "error")
		http.Redirect(w, r, "/admin/promo-codes", http.StatusSeeOther)
		return
	}
	if err := handler.promoSrv.Create(r.Context(), c); err != nil {
		if errors.Is(err, promoapp.ErrCodeAlreadyExists) {
			handler.flash(w, r, "A promo code with that name already exists.", "error")
		} else {
			handler.flash(w, r, err.Error(), "error")
		}
	} else {
		handler.flash(w, r, "Promo code created", "info")
	}
	http.Redirect(w, r, "/admin/promo-codes", http.StatusSeeOther)
}

// AdminEditPromoCodeForm renders the edit form for a single promo code.
func (handler httpHandler) AdminEditPromoCodeForm(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	code := mux.Vars(r)["code"]
	c, err := handler.promoSrv.Find(r.Context(), code)
	if errors.Is(err, promoapp.ErrCodeNotFound) {
		handler.flash(w, r, "Promo code not found", "error")
		http.Redirect(w, r, "/admin/promo-codes", http.StatusSeeOther)
		return
	}
	if err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	handler.renderAdminTemplate(w, r, "admin/promo_code_edit", map[string]any{
		"Active": "promo-codes",
		"Email":  email,
		"Code":   c,
	})
}

// AdminUpdatePromoCode handles the edit-promo-code form submission. The
// code text (PK) is fixed; everything else is editable.
func (handler httpHandler) AdminUpdatePromoCode(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	code := mux.Vars(r)["code"]
	c, err := promoCodeFromForm(r, code)
	if err != nil {
		handler.flash(w, r, "Invalid promo code: "+err.Error(), "error")
		http.Redirect(w, r, "/admin/promo-codes/"+code+"/edit", http.StatusSeeOther)
		return
	}
	if err := handler.promoSrv.Update(r.Context(), c); err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/promo-codes/"+code+"/edit", http.StatusSeeOther)
		return
	}
	handler.flash(w, r, "Promo code updated", "info")
	http.Redirect(w, r, "/admin/promo-codes", http.StatusSeeOther)
}

// AdminDeletePromoCode deletes a promo code (the cascade on
// promo_redemption keeps the per-order ledger consistent).
func (handler httpHandler) AdminDeletePromoCode(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	code := mux.Vars(r)["code"]
	if err := handler.promoSrv.Delete(r.Context(), code); err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Promo code deleted", "info")
	}
	http.Redirect(w, r, "/admin/promo-codes", http.StatusSeeOther)
}

// promoCodeFromForm parses the admin form into a domain.Code. The code
// text is supplied separately so the same helper drives Create (form
// value) and Update (URL path variable).
func promoCodeFromForm(r *http.Request, code string) (promodomain.Code, error) {
	kind := promodomain.Kind(strings.TrimSpace(r.FormValue("kind")))
	value, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("value")), 10, 64)
	currency := strings.TrimSpace(r.FormValue("currency"))
	from := parseOptionalDate(r.FormValue("valid_from"))
	until := parseOptionalDate(r.FormValue("valid_until"))
	maxUses, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("max_uses")))
	perCustomerMax, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("per_customer_max")))
	return promodomain.NewCode(code, kind, value, currency, from, until, maxUses, perCustomerMax)
}

// parseOptionalDate accepts an empty string (meaning "no bound") or a
// YYYY-MM-DD HTML date input, returning a *time.Time in UTC.
func parseOptionalDate(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil
	}
	t = t.UTC()
	return &t
}
