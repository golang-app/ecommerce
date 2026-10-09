package layout

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	widgetdomain "github.com/bkielbasa/go-ecommerce/backend/widget/domain"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// AdminWidgets renders the widget list with optional filtering by page parameter.
func (handler httpHandler) AdminWidgets(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}

	var widgets []widgetdomain.Widget
	if handler.widgetSrv != nil {
		wList, err := handler.widgetSrv.ListAll(r.Context())
		if err != nil {
			handler.flash(w, r, err.Error(), "error")
		}
		widgets = wList
	}

	filterPage := strings.TrimSpace(r.URL.Query().Get("page"))
	var filtered []widgetdomain.Widget
	if filterPage != "" && filterPage != "all" {
		for _, wItem := range widgets {
			if string(wItem.Page) == filterPage {
				filtered = append(filtered, wItem)
			}
		}
	} else {
		filtered = widgets
	}

	handler.renderAdminTemplate(w, r, "admin/widgets", map[string]any{
		"Active":     "widgets",
		"Email":      email,
		"Widgets":    filtered,
		"FilterPage": filterPage,
	})
}

// AdminNewWidget renders the form to create a new widget.
func (handler httpHandler) AdminNewWidget(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}

	var categories any
	if handler.catalogSrv != nil {
		cats, err := handler.catalogSrv.Categories(r.Context())
		if err == nil {
			categories = cats
		}
	}

	defaultWidget := widgetdomain.Widget{
		IsActive:     true,
		DisplayOrder: 1,
	}

	handler.renderAdminTemplate(w, r, "admin/widget_edit", map[string]any{
		"Active":       "widgets",
		"Email":        email,
		"IsNew":        true,
		"Widget":       defaultWidget,
		"Categories":   categories,
		"ProductLimit": 8,
	})
}

// AdminCreateWidget handles the form submission to create a new widget.
func (handler httpHandler) AdminCreateWidget(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}

	_ = r.ParseMultipartForm(maxMultipartSize)

	page := widgetdomain.Page(r.FormValue("page"))
	zone := widgetdomain.Zone(r.FormValue("zone"))
	wType := widgetdomain.Type(r.FormValue("type"))
	title := strings.TrimSpace(r.FormValue("title"))
	displayOrder, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("display_order")))
	isActive := r.FormValue("is_active") == "on" || r.FormValue("is_active") == "true" || r.FormValue("is_active") == "1"

	id := uuid.New().String()
	wDomain, err := widgetdomain.NewWidget(id, page, zone, wType, title, displayOrder)
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/widgets/new", http.StatusSeeOther)
		return
	}
	wDomain.IsActive = isActive

	switch wType {
	case widgetdomain.TypeBannerCarousel:
		cfg := widgetdomain.BannerCarouselConfig{}
		wDomain.Config, _ = json.Marshal(cfg)
	case widgetdomain.TypeNewArrivals:
		limit, _ := strconv.Atoi(r.FormValue("product_limit"))
		if limit <= 0 {
			limit = 8
		}
		cfg := widgetdomain.NewArrivalsConfig{ProductLimit: limit}
		wDomain.Config, _ = json.Marshal(cfg)
	case widgetdomain.TypeCategoryProducts:
		catID := strings.TrimSpace(r.FormValue("category_id"))
		limit, _ := strconv.Atoi(r.FormValue("product_limit"))
		if limit <= 0 {
			limit = 8
		}
		cfg := widgetdomain.CategoryProductsConfig{CategoryID: catID, ProductLimit: limit}
		wDomain.Config, _ = json.Marshal(cfg)
	}

	if handler.widgetSrv != nil {
		if err := handler.widgetSrv.Create(r.Context(), wDomain); err != nil {
			handler.flash(w, r, err.Error(), "error")
			http.Redirect(w, r, "/admin/widgets/new", http.StatusSeeOther)
			return
		}
	}

	handler.flash(w, r, "Widget created", "info")
	http.Redirect(w, r, "/admin/widgets/"+id+"/edit", http.StatusSeeOther)
}

// AdminEditWidget renders the form to edit an existing widget.
func (handler httpHandler) AdminEditWidget(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}

	id := mux.Vars(r)["id"]
	if handler.widgetSrv == nil {
		http.Redirect(w, r, "/admin/widgets", http.StatusSeeOther)
		return
	}

	wDomain, err := handler.widgetSrv.Get(r.Context(), id)
	if err != nil {
		handler.flash(w, r, "Widget not found", "error")
		http.Redirect(w, r, "/admin/widgets", http.StatusSeeOther)
		return
	}

	var categories any
	if handler.catalogSrv != nil {
		cats, err := handler.catalogSrv.Categories(r.Context())
		if err == nil {
			categories = cats
		}
	}

	var productLimit int
	var categoryID string

	switch wDomain.Type {
	case widgetdomain.TypeNewArrivals:
		var cfg widgetdomain.NewArrivalsConfig
		if err := json.Unmarshal(wDomain.Config, &cfg); err == nil {
			productLimit = cfg.ProductLimit
		}
	case widgetdomain.TypeCategoryProducts:
		var cfg widgetdomain.CategoryProductsConfig
		if err := json.Unmarshal(wDomain.Config, &cfg); err == nil {
			productLimit = cfg.ProductLimit
			categoryID = cfg.CategoryID
		}
	}

	handler.renderAdminTemplate(w, r, "admin/widget_edit", map[string]any{
		"Active":       "widgets",
		"Email":        email,
		"IsNew":        false,
		"Widget":       wDomain,
		"Categories":   categories,
		"ProductLimit": productLimit,
		"CategoryID":   categoryID,
	})
}

// AdminUpdateWidget handles updating an existing widget.
func (handler httpHandler) AdminUpdateWidget(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}

	id := mux.Vars(r)["id"]
	if handler.widgetSrv == nil {
		http.Redirect(w, r, "/admin/widgets", http.StatusSeeOther)
		return
	}

	wDomain, err := handler.widgetSrv.Get(r.Context(), id)
	if err != nil {
		handler.flash(w, r, "Widget not found", "error")
		http.Redirect(w, r, "/admin/widgets", http.StatusSeeOther)
		return
	}

	_ = r.ParseMultipartForm(maxMultipartSize)

	wDomain.Page = widgetdomain.Page(r.FormValue("page"))
	wDomain.Zone = widgetdomain.Zone(r.FormValue("zone"))
	wDomain.Title = strings.TrimSpace(r.FormValue("title"))
	wDomain.DisplayOrder, _ = strconv.Atoi(strings.TrimSpace(r.FormValue("display_order")))
	wDomain.IsActive = r.FormValue("is_active") == "on" || r.FormValue("is_active") == "true" || r.FormValue("is_active") == "1"

	switch wDomain.Type {
	case widgetdomain.TypeNewArrivals:
		limit, _ := strconv.Atoi(r.FormValue("product_limit"))
		if limit <= 0 {
			limit = 8
		}
		cfg := widgetdomain.NewArrivalsConfig{ProductLimit: limit}
		wDomain.Config, _ = json.Marshal(cfg)
	case widgetdomain.TypeCategoryProducts:
		catID := strings.TrimSpace(r.FormValue("category_id"))
		limit, _ := strconv.Atoi(r.FormValue("product_limit"))
		if limit <= 0 {
			limit = 8
		}
		cfg := widgetdomain.CategoryProductsConfig{CategoryID: catID, ProductLimit: limit}
		wDomain.Config, _ = json.Marshal(cfg)
	}

	if err := handler.widgetSrv.Update(r.Context(), wDomain); err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/widgets/"+id+"/edit", http.StatusSeeOther)
		return
	}

	handler.flash(w, r, "Widget updated", "info")
	http.Redirect(w, r, "/admin/widgets/"+id+"/edit", http.StatusSeeOther)
}

// AdminToggleWidget toggles active status of a widget.
func (handler httpHandler) AdminToggleWidget(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}

	id := mux.Vars(r)["id"]
	if handler.widgetSrv != nil {
		if err := handler.widgetSrv.ToggleActive(r.Context(), id); err != nil {
			handler.flash(w, r, err.Error(), "error")
		}
	}

	http.Redirect(w, r, "/admin/widgets", http.StatusSeeOther)
}

// AdminDeleteWidget deletes a widget.
func (handler httpHandler) AdminDeleteWidget(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}

	id := mux.Vars(r)["id"]
	if handler.widgetSrv != nil {
		if err := handler.widgetSrv.Delete(r.Context(), id); err != nil {
			handler.flash(w, r, err.Error(), "error")
		} else {
			handler.flash(w, r, "Widget deleted", "info")
		}
	}

	http.Redirect(w, r, "/admin/widgets", http.StatusSeeOther)
}

// AdminAddWidgetSlide adds a slide to a banner carousel widget.
func (handler httpHandler) AdminAddWidgetSlide(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}

	_ = r.ParseMultipartForm(maxMultipartSize)

	widgetID := mux.Vars(r)["id"]
	slideID := uuid.New().String()

	s, err := widgetdomain.NewSlide(slideID, widgetID)
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/widgets/"+widgetID+"/edit", http.StatusSeeOther)
		return
	}

	s.Title = strings.TrimSpace(r.FormValue("title"))
	s.Subtitle = strings.TrimSpace(r.FormValue("subtitle"))
	s.LinkURL = strings.TrimSpace(r.FormValue("link_url"))
	s.LinkText = strings.TrimSpace(r.FormValue("link_text"))
	s.PromoCode = strings.TrimSpace(r.FormValue("promo_code"))
	s.Badge = strings.TrimSpace(r.FormValue("badge"))
	s.DisplayOrder, _ = strconv.Atoi(strings.TrimSpace(r.FormValue("display_order")))

	imgURL, err := handler.resolveImage(r, "image_file", "image_url")
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/widgets/"+widgetID+"/edit", http.StatusSeeOther)
		return
	}
	if imgURL != "" {
		s.ImageURL = imgURL
	}

	if handler.widgetSrv != nil {
		if err := handler.widgetSrv.AddSlide(r.Context(), s); err != nil {
			handler.flash(w, r, err.Error(), "error")
			http.Redirect(w, r, "/admin/widgets/"+widgetID+"/edit", http.StatusSeeOther)
			return
		}
	}

	handler.flash(w, r, "Slide added", "info")
	http.Redirect(w, r, "/admin/widgets/"+widgetID+"/edit", http.StatusSeeOther)
}

// AdminDeleteWidgetSlide deletes a slide from a widget.
func (handler httpHandler) AdminDeleteWidgetSlide(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}

	widgetID := mux.Vars(r)["id"]
	slideID := mux.Vars(r)["slideID"]

	if handler.widgetSrv != nil {
		if err := handler.widgetSrv.DeleteSlide(r.Context(), widgetID, slideID); err != nil {
			handler.flash(w, r, err.Error(), "error")
		} else {
			handler.flash(w, r, "Slide deleted", "info")
		}
	}

	http.Redirect(w, r, "/admin/widgets/"+widgetID+"/edit", http.StatusSeeOther)
}
