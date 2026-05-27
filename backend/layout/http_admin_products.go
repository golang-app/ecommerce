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
	handler.renderAdminTemplate(w, r, "admin/products", map[string]any{
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

	handler.renderAdminTemplate(w, r, "admin/product_edit", map[string]any{
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

// parseOptionsSpec parses an "Name=Value, Name2=Value2" string into a map.
// Empty pairs and pairs without an "=" are skipped; keys/values are trimmed.
func parseOptionsSpec(s string) map[string]string {
	out := map[string]string{}
	for _, pair := range strings.Split(s, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		eq := strings.Index(pair, "=")
		if eq < 0 {
			continue
		}
		k := strings.TrimSpace(pair[:eq])
		v := strings.TrimSpace(pair[eq+1:])
		if k == "" || v == "" {
			continue
		}
		out[k] = v
	}
	return out
}

// AdminNewVariantProductForm renders the create-a-variant-product page.
func (handler httpHandler) AdminNewVariantProductForm(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	handler.renderAdminTemplate(w, r, "admin/product_new_variant", map[string]any{
		"Active": "products",
		"Email":  email,
	})
}

// AdminCreateVariantProduct builds option types and variants from the parallel
// form arrays and creates a variant product.
func (handler httpHandler) AdminCreateVariantProduct(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	_ = r.ParseForm()
	id := strings.TrimSpace(r.FormValue("id"))
	currency := strings.TrimSpace(r.FormValue("currency"))
	if currency == "" {
		currency = "USD"
	}

	const back = "/admin/products/new-variant"

	// Option types from ot_name[] / ot_values[].
	otNames := r.Form["ot_name[]"]
	otValues := r.Form["ot_values[]"]
	var optionTypes []pcapp.OptionTypeInput
	declared := map[string]bool{}
	for i, name := range otNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var raw string
		if i < len(otValues) {
			raw = otValues[i]
		}
		var values []string
		for _, v := range strings.Split(raw, ",") {
			v = strings.TrimSpace(v)
			if v != "" {
				values = append(values, v)
			}
		}
		optionTypes = append(optionTypes, pcapp.OptionTypeInput{Name: name, Values: values})
		declared[name] = true
	}
	if len(optionTypes) == 0 {
		handler.flash(w, r, "at least one option type is required", "error")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}

	// Variants from the parallel v_*[] arrays.
	vSKUs := r.Form["v_sku[]"]
	vPrices := r.Form["v_price[]"]
	vStocks := r.Form["v_stock[]"]
	vImages := r.Form["v_image[]"]
	vOptions := r.Form["v_options[]"]
	var variants []pcapp.VariantInput
	for i := range vPrices {
		sku := ""
		if i < len(vSKUs) {
			sku = strings.TrimSpace(vSKUs[i])
		}
		image := ""
		if i < len(vImages) {
			image = strings.TrimSpace(vImages[i])
		}
		optsRaw := ""
		if i < len(vOptions) {
			optsRaw = vOptions[i]
		}
		options := parseOptionsSpec(optsRaw)
		// Skip wholly-empty rows.
		if sku == "" && strings.TrimSpace(vPrices[i]) == "" && len(options) == 0 {
			continue
		}
		price, err := parsePriceMinorUnits(vPrices[i])
		if err != nil {
			handler.flash(w, r, err.Error(), "error")
			http.Redirect(w, r, back, http.StatusSeeOther)
			return
		}
		stock := 0
		if i < len(vStocks) {
			if raw := strings.TrimSpace(vStocks[i]); raw != "" {
				s, convErr := strconv.Atoi(raw)
				if convErr != nil {
					handler.flash(w, r, "invalid stock", "error")
					http.Redirect(w, r, back, http.StatusSeeOther)
					return
				}
				stock = s
			}
		}
		// Validate the variant's option keys match the declared option types.
		for k := range options {
			if !declared[k] {
				handler.flash(w, r, "variant option \""+k+"\" is not a declared option type", "error")
				http.Redirect(w, r, back, http.StatusSeeOther)
				return
			}
		}
		variantID := slugifyForID(sku)
		if variantID == "" {
			variantID = id + "-" + strconv.Itoa(i)
		}
		variants = append(variants, pcapp.VariantInput{
			ID:      variantID,
			SKU:     sku,
			Image:   image,
			Options: options,
			Price:   price,
			Stock:   stock,
		})
	}
	if len(variants) == 0 {
		handler.flash(w, r, "at least one variant is required", "error")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}

	err := handler.catalogSrv.AddVariantProduct(r.Context(), id, r.FormValue("name"), r.FormValue("description"), currency, r.FormValue("thumbnail"), optionTypes, variants)
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	handler.flash(w, r, "Variant product created", "info")
	http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
}

// slugifyForID lowercases and url-safes a string for use as a variant id. It
// mirrors the app-layer slug rules (lowercase letters/digits, hyphen-separated).
func slugifyForID(s string) string {
	var b strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		case r == ' ' || r == '-' || r == '_':
			if !lastHyphen && b.Len() > 0 {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// AdminAddVariant adds a single variant to an existing product. It reads one
// opt_<OptionTypeName> value per option type plus sku/price/stock/image.
func (handler httpHandler) AdminAddVariant(w http.ResponseWriter, r *http.Request) {
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
	options := map[string]string{}
	for _, ot := range product.OptionTypes() {
		options[ot.Name()] = strings.TrimSpace(r.FormValue("opt_" + ot.Name()))
	}
	currency := strings.TrimSpace(r.FormValue("currency"))
	if currency == "" {
		currency = product.Price().Currency().String()
	}
	price, err := parsePriceMinorUnits(r.FormValue("price"))
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
		return
	}
	stock := 0
	if raw := strings.TrimSpace(r.FormValue("stock")); raw != "" {
		s, convErr := strconv.Atoi(raw)
		if convErr != nil {
			handler.flash(w, r, "invalid stock", "error")
			http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
			return
		}
		stock = s
	}
	err = handler.catalogSrv.AddVariantToProduct(r.Context(), id, r.FormValue("sku"), r.FormValue("image"), price, currency, stock, options)
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Variant added", "info")
	}
	http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
}

// AdminUpdateVariant updates a single variant's sku/price/stock/image.
func (handler httpHandler) AdminUpdateVariant(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	variantID := mux.Vars(r)["variantID"]
	product, err := handler.catalogSrv.Find(r.Context(), id)
	if err != nil {
		handler.flash(w, r, "Product not found", "error")
		http.Redirect(w, r, "/admin/products", http.StatusSeeOther)
		return
	}
	_ = r.ParseForm()
	currency := strings.TrimSpace(r.FormValue("currency"))
	if currency == "" {
		currency = product.Price().Currency().String()
	}
	price, err := parsePriceMinorUnits(r.FormValue("price"))
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
		return
	}
	stock := 0
	if raw := strings.TrimSpace(r.FormValue("stock")); raw != "" {
		s, convErr := strconv.Atoi(raw)
		if convErr != nil {
			handler.flash(w, r, "invalid stock", "error")
			http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
			return
		}
		stock = s
	}
	err = handler.catalogSrv.UpdateVariant(r.Context(), variantID, r.FormValue("sku"), r.FormValue("image"), price, currency, stock)
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Variant updated", "info")
	}
	http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
}

// AdminDeleteVariant deletes a single variant (refused if it is the last one).
func (handler httpHandler) AdminDeleteVariant(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	variantID := mux.Vars(r)["variantID"]
	if err := handler.catalogSrv.DeleteVariant(r.Context(), id, variantID); err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Variant deleted", "info")
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
