package layout

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	pcapp "github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/gorilla/mux"
)

// parsePriceMinorUnits parses a decimal major-units price string (e.g. "9.00"
// or "9.5") into integer minor units (cents). It rejects negative or malformed
// input.
func parsePriceMinorUnits(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("price is required")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, errors.New("price must be a number (major units, e.g. 9.00)")
	}
	if f < 0 {
		return 0, errors.New("price cannot be negative")
	}
	return int64(math.Round(f * 100)), nil
}

// AdminProducts renders the product list page.
func (handler httpHandler) AdminProducts(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	products, err := handler.catalogSrv.AllProducts(r.Context())
	if err != nil {
		products = nil
	}
	handler.renderTemplate(w, r, "admin/products", map[string]any{
		"Active":   "products",
		"Email":    email,
		"Products": products,
	})
}

// AdminCreateProduct handles the create-product form (a simple product with a
// single default variant). Price is entered in major units (e.g. "9.00").
func (handler httpHandler) AdminCreateProduct(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	_ = r.ParseForm()
	id := strings.TrimSpace(r.FormValue("id"))
	currency := strings.TrimSpace(r.FormValue("currency"))
	if currency == "" {
		currency = "USD"
	}
	price, err := parsePriceMinorUnits(r.FormValue("price"))
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/products", http.StatusSeeOther)
		return
	}
	err = handler.catalogSrv.Add(r.Context(), id, r.FormValue("name"), r.FormValue("description"), price, currency, r.FormValue("thumbnail"))
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/products", http.StatusSeeOther)
		return
	}
	handler.flash(w, r, "Product created", "info")
	http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
}

// AdminEditProductForm renders the multi-section product edit page.
func (handler httpHandler) AdminEditProductForm(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	id := mux.Vars(r)["id"]
	product, err := handler.catalogSrv.Find(r.Context(), id)
	if err != nil {
		handler.flash(w, r, "Product not found", "error")
		http.Redirect(w, r, "/admin/products", http.StatusSeeOther)
		return
	}

	categories, err := handler.catalogSrv.Categories(r.Context())
	if err != nil {
		categories = nil
	}
	assigned := map[string]bool{}
	for _, c := range product.Categories() {
		assigned[c.ID()] = true
	}

	attrTypes, err := handler.catalogSrv.AttributeTypes(r.Context())
	if err != nil {
		attrTypes = nil
	}
	// Current value per attribute-type id, for prefilling the inputs.
	attrValues := map[string]string{}
	for _, av := range product.Attributes() {
		if av.Type().IsNumeric() {
			attrValues[av.Type().ID()] = strconv.FormatFloat(av.NumValue(), 'f', -1, 64)
		} else {
			attrValues[av.Type().ID()] = av.TextValue()
		}
	}

	handler.renderTemplate(w, r, "admin/product_edit", map[string]any{
		"Active":     "products",
		"Email":      email,
		"Product":    product,
		"Categories": categories,
		"Assigned":   assigned,
		"AttrTypes":  attrTypes,
		"AttrValues": attrValues,
	})
}

// AdminUpdateProduct handles the core-fields form on the edit page.
func (handler httpHandler) AdminUpdateProduct(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	_ = r.ParseForm()
	currency := strings.TrimSpace(r.FormValue("currency"))
	if currency == "" {
		currency = "USD"
	}
	price, err := parsePriceMinorUnits(r.FormValue("price"))
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
		return
	}
	err = handler.catalogSrv.UpdateProduct(r.Context(), id, r.FormValue("name"), r.FormValue("description"), price, currency, r.FormValue("thumbnail"))
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Product updated", "info")
	}
	http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
}

// AdminUpdateProductStock parses each variant's stock input (stock_<variantID>)
// and persists it.
func (handler httpHandler) AdminUpdateProductStock(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	product, err := handler.catalogSrv.Find(r.Context(), id)
	if err != nil {
		handler.flash(w, r, "Product not found", "error")
		http.Redirect(w, r, "/admin/products", http.StatusSeeOther)
		return
	}
	_ = r.ParseForm()
	for _, v := range product.Variants() {
		raw := strings.TrimSpace(r.FormValue("stock_" + v.ID()))
		if raw == "" {
			continue
		}
		stock, convErr := strconv.Atoi(raw)
		if convErr != nil {
			handler.flash(w, r, "Invalid stock for "+v.ID(), "error")
			http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
			return
		}
		if err = handler.catalogSrv.SetVariantStock(r.Context(), v.ID(), stock); err != nil {
			handler.flash(w, r, err.Error(), "error")
			http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
			return
		}
	}
	handler.flash(w, r, "Stock updated", "info")
	http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
}

// AdminUpdateProductCategories replaces the product's category assignments with
// the checked boxes.
func (handler httpHandler) AdminUpdateProductCategories(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	_ = r.ParseForm()
	categoryIDs := r.Form["category"]
	if err := handler.catalogSrv.SetProductCategories(r.Context(), id, categoryIDs); err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Categories updated", "info")
	}
	http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
}

// AdminUpdateProductAttributes reads one input per attribute type
// (attr_<typeID>); a non-empty value becomes an assignment (numeric values are
// parsed as floats, enum values stored verbatim). Empty inputs are omitted,
// i.e. cleared.
func (handler httpHandler) AdminUpdateProductAttributes(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	attrTypes, err := handler.catalogSrv.AttributeTypes(r.Context())
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
		return
	}
	_ = r.ParseForm()
	var values []pcapp.AttributeAssignment
	for _, t := range attrTypes {
		raw := strings.TrimSpace(r.FormValue("attr_" + t.ID()))
		if raw == "" {
			continue
		}
		if t.IsNumeric() {
			f, convErr := strconv.ParseFloat(raw, 64)
			if convErr != nil {
				handler.flash(w, r, "Invalid number for "+t.Name(), "error")
				http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
				return
			}
			values = append(values, pcapp.AttributeAssignment{TypeID: t.ID(), Num: &f})
		} else {
			values = append(values, pcapp.AttributeAssignment{TypeID: t.ID(), Text: raw})
		}
	}
	if err := handler.catalogSrv.SetProductAttributes(r.Context(), id, values); err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Attributes updated", "info")
	}
	http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
}

// AdminDeleteProduct deletes a product (variants/links cascade).
func (handler httpHandler) AdminDeleteProduct(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	if err := handler.catalogSrv.DeleteProduct(r.Context(), id); err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Product deleted", "info")
	}
	http.Redirect(w, r, "/admin/products", http.StatusSeeOther)
}
