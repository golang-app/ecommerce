package layout

import (
	"net/http"
	"strconv"

	pcdomain "github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	"github.com/gorilla/mux"
)

// AdminCategories renders the categories list page with the inline "new" form.
func (handler httpHandler) AdminCategories(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	categories, err := handler.catalogSrv.Categories(r.Context())
	if err != nil {
		categories = nil
	}
	handler.renderAdminTemplate(w, r, "admin/categories", map[string]any{
		"Active":     "categories",
		"Email":      email,
		"Categories": categories,
	})
}

// AdminCreateCategory handles the create-category form submission.
func (handler httpHandler) AdminCreateCategory(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	_ = r.ParseForm()
	err := handler.catalogSrv.CreateCategory(r.Context(), r.FormValue("name"), r.FormValue("slug"))
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Category created", "info")
	}
	http.Redirect(w, r, "/admin/categories", http.StatusSeeOther)
}

// AdminEditCategoryForm renders the edit form for a single category.
func (handler httpHandler) AdminEditCategoryForm(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	id := mux.Vars(r)["id"]
	categories, err := handler.catalogSrv.Categories(r.Context())
	if err != nil {
		http.Redirect(w, r, "/admin/categories", http.StatusSeeOther)
		return
	}
	var found *pcdomain.Category
	for i := range categories {
		if categories[i].ID() == id {
			found = &categories[i]
			break
		}
	}
	if found == nil {
		handler.flash(w, r, "Category not found", "error")
		http.Redirect(w, r, "/admin/categories", http.StatusSeeOther)
		return
	}
	handler.renderAdminTemplate(w, r, "admin/category_edit", map[string]any{
		"Active":   "categories",
		"Email":    email,
		"Category": *found,
	})
}

// AdminUpdateCategory handles the edit-category form submission.
func (handler httpHandler) AdminUpdateCategory(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	_ = r.ParseForm()
	position, _ := strconv.Atoi(r.FormValue("position"))
	err := handler.catalogSrv.UpdateCategory(r.Context(), id, r.FormValue("name"), r.FormValue("slug"), position)
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/categories/"+id+"/edit", http.StatusSeeOther)
		return
	}
	handler.flash(w, r, "Category updated", "info")
	http.Redirect(w, r, "/admin/categories", http.StatusSeeOther)
}

// AdminDeleteCategory deletes a category (and cascades its product links).
func (handler httpHandler) AdminDeleteCategory(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	if err := handler.catalogSrv.DeleteCategory(r.Context(), id); err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Category deleted", "info")
	}
	http.Redirect(w, r, "/admin/categories", http.StatusSeeOther)
}

// AdminAttributes renders the attribute types list page with the inline form.
func (handler httpHandler) AdminAttributes(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	types, err := handler.catalogSrv.AttributeTypes(r.Context())
	if err != nil {
		types = nil
	}
	handler.renderAdminTemplate(w, r, "admin/attributes", map[string]any{
		"Active":     "attributes",
		"Email":      email,
		"Attributes": types,
	})
}

// AdminCreateAttribute handles the create-attribute-type form submission.
func (handler httpHandler) AdminCreateAttribute(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	_ = r.ParseForm()
	kind := pcdomain.AttributeKind(r.FormValue("kind"))
	filterable := r.FormValue("filterable") != ""
	err := handler.catalogSrv.CreateAttributeType(r.Context(), r.FormValue("name"), r.FormValue("unit"), kind, filterable)
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Attribute type created", "info")
	}
	http.Redirect(w, r, "/admin/attributes", http.StatusSeeOther)
}

// AdminEditAttributeForm renders the edit form for a single attribute type.
func (handler httpHandler) AdminEditAttributeForm(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	id := mux.Vars(r)["id"]
	types, err := handler.catalogSrv.AttributeTypes(r.Context())
	if err != nil {
		http.Redirect(w, r, "/admin/attributes", http.StatusSeeOther)
		return
	}
	var found *pcdomain.AttributeType
	for i := range types {
		if types[i].ID() == id {
			found = &types[i]
			break
		}
	}
	if found == nil {
		handler.flash(w, r, "Attribute type not found", "error")
		http.Redirect(w, r, "/admin/attributes", http.StatusSeeOther)
		return
	}
	handler.renderAdminTemplate(w, r, "admin/attribute_edit", map[string]any{
		"Active":    "attributes",
		"Email":     email,
		"Attribute": *found,
	})
}

// AdminUpdateAttribute handles the edit-attribute-type form submission.
func (handler httpHandler) AdminUpdateAttribute(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	_ = r.ParseForm()
	position, _ := strconv.Atoi(r.FormValue("position"))
	kind := pcdomain.AttributeKind(r.FormValue("kind"))
	filterable := r.FormValue("filterable") != ""
	err := handler.catalogSrv.UpdateAttributeType(r.Context(), id, r.FormValue("name"), r.FormValue("unit"), kind, filterable, position)
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/attributes/"+id+"/edit", http.StatusSeeOther)
		return
	}
	handler.flash(w, r, "Attribute type updated", "info")
	http.Redirect(w, r, "/admin/attributes", http.StatusSeeOther)
}

// AdminDeleteAttribute deletes an attribute type (and cascades product links).
func (handler httpHandler) AdminDeleteAttribute(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	if err := handler.catalogSrv.DeleteAttributeType(r.Context(), id); err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Attribute type deleted", "info")
	}
	http.Redirect(w, r, "/admin/attributes", http.StatusSeeOther)
}
