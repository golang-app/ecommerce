package layout

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	storeapp "github.com/bkielbasa/go-ecommerce/backend/store/app"
	storeDomain "github.com/bkielbasa/go-ecommerce/backend/store/domain"
	"github.com/gorilla/mux"
)

// AdminStores renders the stores list page with the inline create
// form. Admin-gated like every other /admin route.
func (handler httpHandler) AdminStores(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	var stores []storeDomain.Store
	if handler.storeSrv != nil {
		s, err := handler.storeSrv.ListAll(r.Context())
		if err != nil {
			handler.flash(w, r, err.Error(), "error")
		}
		stores = s
	}
	handler.renderAdminTemplate(w, r, "admin/stores", map[string]any{
		"Active": "stores",
		"Email":  email,
		"Stores": stores,
	})
}

// AdminCreateStore handles the create-store form submission. Validation
// errors flash and bounce back to the list page; success flashes and
// redirects too.
func (handler httpHandler) AdminCreateStore(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	s, err := storeFromForm(r, r.FormValue("id"))
	if err != nil {
		handler.flash(w, r, "Invalid store: "+err.Error(), "error")
		http.Redirect(w, r, "/admin/stores", http.StatusSeeOther)
		return
	}
	if handler.storeSrv == nil {
		handler.flash(w, r, "store service not wired", "error")
		http.Redirect(w, r, "/admin/stores", http.StatusSeeOther)
		return
	}
	if err := handler.storeSrv.Create(r.Context(), s); err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Store created", "info")
	}
	http.Redirect(w, r, "/admin/stores", http.StatusSeeOther)
}

// AdminEditStoreForm renders the edit form for a single store.
func (handler httpHandler) AdminEditStoreForm(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	if handler.storeSrv == nil {
		http.Redirect(w, r, "/admin/stores", http.StatusSeeOther)
		return
	}
	id := mux.Vars(r)["id"]
	s, err := handler.storeSrv.Find(r.Context(), id)
	if errors.Is(err, storeapp.ErrStoreNotFound) {
		handler.flash(w, r, "Store not found", "error")
		http.Redirect(w, r, "/admin/stores", http.StatusSeeOther)
		return
	}
	if err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	handler.renderAdminTemplate(w, r, "admin/store_edit", map[string]any{
		"Active": "stores",
		"Email":  email,
		"Store":  s,
	})
}

// AdminUpdateStore handles the edit-store form submission. The id (PK)
// is fixed by the URL; everything else is editable.
func (handler httpHandler) AdminUpdateStore(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	if handler.storeSrv == nil {
		http.Redirect(w, r, "/admin/stores", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	id := mux.Vars(r)["id"]
	s, err := storeFromForm(r, id)
	if err != nil {
		handler.flash(w, r, "Invalid store: "+err.Error(), "error")
		http.Redirect(w, r, "/admin/stores/"+id+"/edit", http.StatusSeeOther)
		return
	}
	if err := handler.storeSrv.Update(r.Context(), s); err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/stores/"+id+"/edit", http.StatusSeeOther)
		return
	}
	handler.flash(w, r, "Store updated", "info")
	http.Redirect(w, r, "/admin/stores", http.StatusSeeOther)
}

// AdminDeleteStore drops a store row.
func (handler httpHandler) AdminDeleteStore(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	if handler.storeSrv == nil {
		http.Redirect(w, r, "/admin/stores", http.StatusSeeOther)
		return
	}
	id := mux.Vars(r)["id"]
	if err := handler.storeSrv.Delete(r.Context(), id); err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Store deleted", "info")
	}
	http.Redirect(w, r, "/admin/stores", http.StatusSeeOther)
}

// storeFromForm parses the admin form into a domain.Store. The id is
// supplied separately so the same helper drives Create (form value)
// and Update (URL path variable).
func storeFromForm(r *http.Request, id string) (storeDomain.Store, error) {
	slug := strings.TrimSpace(r.FormValue("slug"))
	name := strings.TrimSpace(r.FormValue("name"))
	currency := strings.TrimSpace(r.FormValue("currency"))
	host := strings.TrimSpace(r.FormValue("host"))
	isDefault := r.FormValue("is_default") != ""
	position, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("position")))
	return storeDomain.NewStore(strings.TrimSpace(id), slug, name, currency, host, isDefault, position)
}
