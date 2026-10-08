package layout

import (
	"net/http"
	"strconv"
	"strings"

	bannerdomain "github.com/bkielbasa/go-ecommerce/backend/banner/domain"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// AdminBanners renders the list of all banners.
func (handler httpHandler) AdminBanners(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	var banners []bannerdomain.Banner
	if handler.bannerSrv != nil {
		b, err := handler.bannerSrv.ListAll(r.Context())
		if err != nil {
			handler.flash(w, r, err.Error(), "error")
		} else {
			banners = b
		}
	}
	handler.renderAdminTemplate(w, r, "admin/banners", map[string]any{
		"Active":  "banners",
		"Email":   email,
		"Banners": banners,
	})
}

// AdminNewBanner renders the form to create a new banner.
func (handler httpHandler) AdminNewBanner(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	handler.renderAdminTemplate(w, r, "admin/banner_edit", map[string]any{
		"Active": "banners",
		"Email":  email,
		"IsNew":  true,
		"Banner": bannerdomain.Banner{IsActive: true},
	})
}

// AdminCreateBanner processes the creation of a new banner.
func (handler httpHandler) AdminCreateBanner(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	_ = r.ParseMultipartForm(maxMultipartSize)
	if handler.bannerSrv == nil {
		handler.flash(w, r, "banner service not configured", "error")
		http.Redirect(w, r, "/admin/banners/new", http.StatusSeeOther)
		return
	}

	id := uuid.New().String()
	slot := bannerdomain.Slot(strings.TrimSpace(r.FormValue("slot")))
	title := strings.TrimSpace(r.FormValue("title"))
	displayOrder, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("display_order")))

	b, err := bannerdomain.NewBanner(id, slot, title, displayOrder)
	if err != nil {
		handler.flash(w, r, "Invalid banner: "+err.Error(), "error")
		http.Redirect(w, r, "/admin/banners/new", http.StatusSeeOther)
		return
	}

	b.Body = strings.TrimSpace(r.FormValue("body"))
	b.LinkURL = strings.TrimSpace(r.FormValue("link_url"))
	b.LinkText = strings.TrimSpace(r.FormValue("link_text"))
	b.PromoCode = strings.TrimSpace(r.FormValue("promo_code"))
	b.Icon = strings.TrimSpace(r.FormValue("icon"))
	b.IsActive = r.FormValue("is_active") == "on" || r.FormValue("is_active") == "true" || r.FormValue("is_active") == "1"

	imgURL, err := handler.resolveImage(r, "image_file", "image_url")
	if err != nil {
		handler.flash(w, r, "Image upload failed: "+err.Error(), "error")
		http.Redirect(w, r, "/admin/banners/new", http.StatusSeeOther)
		return
	}
	if imgURL != "" {
		b.ImageURL = imgURL
	}

	if err := handler.bannerSrv.Create(r.Context(), b); err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/banners/new", http.StatusSeeOther)
		return
	}

	handler.flash(w, r, "Banner created", "info")
	http.Redirect(w, r, "/admin/banners", http.StatusSeeOther)
}

// AdminEditBanner renders the edit form for an existing banner.
func (handler httpHandler) AdminEditBanner(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	if handler.bannerSrv == nil {
		handler.flash(w, r, "banner service not configured", "error")
		http.Redirect(w, r, "/admin/banners", http.StatusSeeOther)
		return
	}
	id := mux.Vars(r)["id"]
	b, err := handler.bannerSrv.Get(r.Context(), id)
	if err != nil {
		handler.flash(w, r, "Banner not found", "error")
		http.Redirect(w, r, "/admin/banners", http.StatusSeeOther)
		return
	}

	handler.renderAdminTemplate(w, r, "admin/banner_edit", map[string]any{
		"Active": "banners",
		"Email":  email,
		"IsNew":  false,
		"Banner": b,
	})
}

// AdminUpdateBanner updates an existing banner.
func (handler httpHandler) AdminUpdateBanner(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	if handler.bannerSrv == nil {
		handler.flash(w, r, "banner service not configured", "error")
		http.Redirect(w, r, "/admin/banners", http.StatusSeeOther)
		return
	}
	id := mux.Vars(r)["id"]
	_ = r.ParseMultipartForm(maxMultipartSize)

	b, err := handler.bannerSrv.Get(r.Context(), id)
	if err != nil {
		handler.flash(w, r, "Banner not found", "error")
		http.Redirect(w, r, "/admin/banners", http.StatusSeeOther)
		return
	}

	slot := bannerdomain.Slot(strings.TrimSpace(r.FormValue("slot")))
	title := strings.TrimSpace(r.FormValue("title"))
	if !slot.IsValid() || title == "" {
		handler.flash(w, r, "Slot and title are required and must be valid", "error")
		http.Redirect(w, r, "/admin/banners/"+id+"/edit", http.StatusSeeOther)
		return
	}

	b.Slot = slot
	b.Title = title
	b.Body = strings.TrimSpace(r.FormValue("body"))
	b.LinkURL = strings.TrimSpace(r.FormValue("link_url"))
	b.LinkText = strings.TrimSpace(r.FormValue("link_text"))
	b.PromoCode = strings.TrimSpace(r.FormValue("promo_code"))
	b.Icon = strings.TrimSpace(r.FormValue("icon"))
	displayOrder, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("display_order")))
	b.DisplayOrder = displayOrder
	b.IsActive = r.FormValue("is_active") == "on" || r.FormValue("is_active") == "true" || r.FormValue("is_active") == "1"

	imgURL, err := handler.resolveImage(r, "image_file", "image_url")
	if err != nil {
		handler.flash(w, r, "Image upload failed: "+err.Error(), "error")
		http.Redirect(w, r, "/admin/banners/"+id+"/edit", http.StatusSeeOther)
		return
	}
	if imgURL != "" {
		b.ImageURL = imgURL
	}

	if err := handler.bannerSrv.Update(r.Context(), b); err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/banners/"+id+"/edit", http.StatusSeeOther)
		return
	}

	handler.flash(w, r, "Banner updated", "info")
	http.Redirect(w, r, "/admin/banners", http.StatusSeeOther)
}

// AdminToggleBanner toggles the active status of a banner.
func (handler httpHandler) AdminToggleBanner(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	if handler.bannerSrv == nil {
		handler.flash(w, r, "banner service not configured", "error")
		http.Redirect(w, r, "/admin/banners", http.StatusSeeOther)
		return
	}
	id := mux.Vars(r)["id"]
	if err := handler.bannerSrv.ToggleActive(r.Context(), id); err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Banner status updated", "info")
	}
	http.Redirect(w, r, "/admin/banners", http.StatusSeeOther)
}

// AdminDeleteBanner deletes a banner.
func (handler httpHandler) AdminDeleteBanner(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	if handler.bannerSrv == nil {
		handler.flash(w, r, "banner service not configured", "error")
		http.Redirect(w, r, "/admin/banners", http.StatusSeeOther)
		return
	}
	id := mux.Vars(r)["id"]
	if err := handler.bannerSrv.Delete(r.Context(), id); err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Banner deleted", "info")
	}
	http.Redirect(w, r, "/admin/banners", http.StatusSeeOther)
}
