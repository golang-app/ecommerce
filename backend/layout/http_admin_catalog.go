package layout

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	pcdomain "github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	"github.com/gorilla/mux"
)

// AdminCategoryItem decorates a domain Category with depth and visual tree indentation
// for admin views and selection dropdowns.
type AdminCategoryItem struct {
	pcdomain.Category
	Depth        int
	IndentedName string
}

// buildCategoryTree sorts domain categories depth-first and assigns visual indentation.
func buildCategoryTree(categories []pcdomain.Category) []AdminCategoryItem {
	if len(categories) == 0 {
		return nil
	}

	childrenMap := make(map[string][]pcdomain.Category)
	for _, c := range categories {
		pID := c.ParentID()
		childrenMap[pID] = append(childrenMap[pID], c)
	}

	for pID := range childrenMap {
		sort.Slice(childrenMap[pID], func(i, j int) bool {
			if childrenMap[pID][i].Position() != childrenMap[pID][j].Position() {
				return childrenMap[pID][i].Position() < childrenMap[pID][j].Position()
			}
			if childrenMap[pID][i].Name() != childrenMap[pID][j].Name() {
				return childrenMap[pID][i].Name() < childrenMap[pID][j].Name()
			}
			return childrenMap[pID][i].ID() < childrenMap[pID][j].ID()
		})
	}

	var result []AdminCategoryItem
	var walk func(parentID string, depth int)
	walk = func(parentID string, depth int) {
		children := childrenMap[parentID]
		for _, child := range children {
			var indentedName string
			if depth == 0 {
				indentedName = child.Name()
			} else {
				indentedName = strings.Repeat("  ", depth-1) + "└── " + child.Name()
			}
			result = append(result, AdminCategoryItem{
				Category:     child,
				Depth:        depth,
				IndentedName: indentedName,
			})
			walk(child.ID(), depth+1)
		}
	}

	walk("", 0)

	// Ensure any orphan categories not reached from root are included
	if len(result) < len(categories) {
		visited := make(map[string]bool, len(result))
		for _, item := range result {
			visited[item.ID()] = true
		}
		for _, c := range categories {
			if !visited[c.ID()] {
				result = append(result, AdminCategoryItem{
					Category:     c,
					Depth:        0,
					IndentedName: c.Name(),
				})
			}
		}
	}

	return result
}

// filterValidParentOptions excludes the target category and all of its descendants
// to prevent assigning cyclic parent references.
func filterValidParentOptions(items []AdminCategoryItem, editCategoryID string) []AdminCategoryItem {
	if editCategoryID == "" {
		return items
	}

	excluded := make(map[string]bool)
	excluded[editCategoryID] = true

	childrenMap := make(map[string][]pcdomain.Category)
	for _, item := range items {
		childrenMap[item.ParentID()] = append(childrenMap[item.ParentID()], item.Category)
	}

	var collectDescendants func(id string)
	collectDescendants = func(id string) {
		for _, child := range childrenMap[id] {
			excluded[child.ID()] = true
			collectDescendants(child.ID())
		}
	}
	collectDescendants(editCategoryID)

	var valid []AdminCategoryItem
	for _, item := range items {
		if !excluded[item.ID()] {
			valid = append(valid, item)
		}
	}
	return valid
}

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
	treeItems := buildCategoryTree(categories)
	handler.renderAdminTemplate(w, r, "admin/categories", map[string]any{
		"Active":     "categories",
		"Email":      email,
		"Categories": treeItems,
	})
}

// AdminCreateCategory handles the create-category form submission.
func (handler httpHandler) AdminCreateCategory(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	_ = r.ParseForm()
	parentID := strings.TrimSpace(r.FormValue("parent_id"))
	err := handler.catalogSrv.CreateCategory(r.Context(), r.FormValue("name"), r.FormValue("slug"), parentID)
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
	treeItems := buildCategoryTree(categories)
	parentOptions := filterValidParentOptions(treeItems, id)

	handler.renderAdminTemplate(w, r, "admin/category_edit", map[string]any{
		"Active":        "categories",
		"Email":         email,
		"Category":      *found,
		"ParentOptions": parentOptions,
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
	parentID := strings.TrimSpace(r.FormValue("parent_id"))
	err := handler.catalogSrv.UpdateCategory(r.Context(), id, r.FormValue("name"), r.FormValue("slug"), parentID, position)
	if err != nil {
		if errors.Is(err, pcdomain.ErrCyclicCategoryHierarchy) {
			handler.flash(w, r, "Cannot assign parent: cyclic category hierarchy detected", "error")
		} else {
			handler.flash(w, r, err.Error(), "error")
		}
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
	if handler.demoMode {
		handler.flash(w, r, "Category deletion is disabled in Demo Mode.", "error")
		http.Redirect(w, r, "/admin/categories", http.StatusSeeOther)
		return
	}
	id := mux.Vars(r)["id"]
	if err := handler.catalogSrv.DeleteCategory(r.Context(), id); err != nil {
		if errors.Is(err, pcdomain.ErrCategoryHasChildren) {
			handler.flash(w, r, "Cannot delete category because it has subcategories. Move or delete them first.", "error")
		} else {
			handler.flash(w, r, err.Error(), "error")
		}
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
