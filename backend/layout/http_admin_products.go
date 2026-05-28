package layout

import (
	"errors"
	"math"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	pcapp "github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/gorilla/mux"
)

// maxMultipartSize caps multipart form parsing. The biggest payload is one
// image (5 MiB, enforced again inside the image store) plus the other text
// fields, so we give a small headroom for those.
const maxMultipartSize int64 = 6 << 20

// resolveImage prefers an uploaded file over a typed-in URL: if the form has a
// file at fileField, it is saved through the image store and the resulting
// public URL is returned. Otherwise the trimmed value of urlField is returned.
// Returns ("", nil) when neither is provided.
func (handler httpHandler) resolveImage(r *http.Request, fileField, urlField string) (string, error) {
	f, fh, err := r.FormFile(fileField)
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return strings.TrimSpace(r.FormValue(urlField)), nil
		}
		return "", err
	}
	defer func() { _ = f.Close() }()
	return handler.imageStore.Save(r.Context(), fh.Filename, fh.Header.Get("Content-Type"), f)
}

// resolveImageFromHeader is the parallel-array variant of resolveImage used by
// the variant-create form. Caller passes the FileHeader (or nil) and the
// fallback URL string. A nil header returns the trimmed URL.
func (handler httpHandler) resolveImageFromHeader(r *http.Request, fh *multipart.FileHeader, urlValue string) (string, error) {
	if fh == nil {
		return strings.TrimSpace(urlValue), nil
	}
	f, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	return handler.imageStore.Save(r.Context(), fh.Filename, fh.Header.Get("Content-Type"), f)
}

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

// AdminNewProductForm renders the standalone "add a simple product" page.
// The form POSTs to /admin/products (handled by AdminCreateProduct).
func (handler httpHandler) AdminNewProductForm(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	attrSets, err := handler.catalogSrv.AttributeSets(r.Context())
	if err != nil {
		attrSets = nil
	}
	handler.renderAdminTemplate(w, r, "admin/product_new", map[string]any{
		"Active":        "products",
		"Email":         email,
		"AttributeSets": attrSets,
	})
}

// AdminCreateProduct handles the create-product form (a simple product with a
// single default variant). Price is entered in major units (e.g. "9.00").
func (handler httpHandler) AdminCreateProduct(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	_ = r.ParseMultipartForm(maxMultipartSize)
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
	thumbnail, err := handler.resolveImage(r, "thumbnail_file", "thumbnail")
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/products", http.StatusSeeOther)
		return
	}
	err = handler.catalogSrv.Add(r.Context(), id, r.FormValue("name"), r.FormValue("description"), price, currency, thumbnail)
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/products", http.StatusSeeOther)
		return
	}
	if setID := strings.TrimSpace(r.FormValue("attribute_set")); setID != "" {
		if err := handler.catalogSrv.SetProductAttributeSet(r.Context(), id, setID); err != nil {
			handler.flash(w, r, "Product created, but the attribute set could not be assigned: "+err.Error(), "error")
			http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
			return
		}
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

	// The attributes section is driven by the product's attribute set (its
	// ordered members), falling back to all attribute types when it has no set.
	attrTypes, err := handler.catalogSrv.ProductAttributeTypes(r.Context(), id)
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

	// All sets (for the picker) and the product's current set name (if any).
	attrSets, err := handler.catalogSrv.AttributeSets(r.Context())
	if err != nil {
		attrSets = nil
	}
	currentSetID := product.AttributeSetID()
	currentSetName := ""
	if currentSetID != "" {
		if set, err := handler.catalogSrv.FindAttributeSet(r.Context(), currentSetID); err == nil {
			currentSetName = set.Name()
		}
	}

	handler.renderAdminTemplate(w, r, "admin/product_edit", map[string]any{
		"Active":         "products",
		"Email":          email,
		"Product":        product,
		"Categories":     categories,
		"Assigned":       assigned,
		"AttrTypes":      attrTypes,
		"AttrValues":     attrValues,
		"AttributeSets":  attrSets,
		"CurrentSetID":   currentSetID,
		"CurrentSetName": currentSetName,
	})
}

// AdminUpdateProduct handles the core-fields form on the edit page.
func (handler httpHandler) AdminUpdateProduct(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	_ = r.ParseMultipartForm(maxMultipartSize)
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
	thumbnail, err := handler.resolveImage(r, "thumbnail_file", "thumbnail")
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
		return
	}
	err = handler.catalogSrv.UpdateProduct(r.Context(), id, r.FormValue("name"), r.FormValue("description"), price, currency, thumbnail)
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
	// Iterate the same ordered attribute types the edit form rendered (the
	// product's set members, or all types when it has no set) so only those are
	// processed/saved.
	attrTypes, err := handler.catalogSrv.ProductAttributeTypes(r.Context(), id)
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

// AdminUpdateProductAttributeSet assigns (or clears) the product's attribute
// set from the picker on the edit page. The set controls which attribute types
// the product's attributes section shows.
func (handler httpHandler) AdminUpdateProductAttributeSet(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	_ = r.ParseForm()
	setID := strings.TrimSpace(r.FormValue("attribute_set"))
	if err := handler.catalogSrv.SetProductAttributeSet(r.Context(), id, setID); err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Attribute set updated", "info")
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
	attrSets, err := handler.catalogSrv.AttributeSets(r.Context())
	if err != nil {
		attrSets = nil
	}
	handler.renderAdminTemplate(w, r, "admin/product_new_variant", map[string]any{
		"Active":        "products",
		"Email":         email,
		"AttributeSets": attrSets,
	})
}

// AdminCreateVariantProduct builds option types and variants from the parallel
// form arrays and creates a variant product.
func (handler httpHandler) AdminCreateVariantProduct(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	_ = r.ParseMultipartForm(maxMultipartSize)
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
	// Parallel uploaded files (may be nil if there is no multipart form).
	var vImageFiles []*multipart.FileHeader
	if r.MultipartForm != nil {
		vImageFiles = r.MultipartForm.File["v_image_file[]"]
	}
	var variants []pcapp.VariantInput
	for i := range vPrices {
		sku := ""
		if i < len(vSKUs) {
			sku = strings.TrimSpace(vSKUs[i])
		}
		urlImage := ""
		if i < len(vImages) {
			urlImage = vImages[i]
		}
		var fh *multipart.FileHeader
		if i < len(vImageFiles) {
			fh = vImageFiles[i]
		}
		image, imgErr := handler.resolveImageFromHeader(r, fh, urlImage)
		if imgErr != nil {
			handler.flash(w, r, imgErr.Error(), "error")
			http.Redirect(w, r, back, http.StatusSeeOther)
			return
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

	thumbnail, err := handler.resolveImage(r, "thumbnail_file", "thumbnail")
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	err = handler.catalogSrv.AddVariantProduct(r.Context(), id, r.FormValue("name"), r.FormValue("description"), currency, thumbnail, optionTypes, variants)
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	if setID := strings.TrimSpace(r.FormValue("attribute_set")); setID != "" {
		if err := handler.catalogSrv.SetProductAttributeSet(r.Context(), id, setID); err != nil {
			handler.flash(w, r, "Variant product created, but the attribute set could not be assigned: "+err.Error(), "error")
			http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
			return
		}
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
	_ = r.ParseMultipartForm(maxMultipartSize)
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
	image, err := handler.resolveImage(r, "image_file", "image")
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
		return
	}
	err = handler.catalogSrv.AddVariantToProduct(r.Context(), id, r.FormValue("sku"), image, price, currency, stock, options)
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
	_ = r.ParseMultipartForm(maxMultipartSize)
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
	image, err := handler.resolveImage(r, "image_file", "image")
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
		return
	}
	err = handler.catalogSrv.UpdateVariant(r.Context(), variantID, r.FormValue("sku"), image, price, currency, stock)
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

// splitCSVValues splits a comma-separated string into trimmed, non-empty values.
func splitCSVValues(raw string) []string {
	var out []string
	for _, v := range strings.Split(raw, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// AdminAddOptionType adds an option type to an existing product. The chosen
// default value is applied to every existing variant so they stay resolvable.
func (handler httpHandler) AdminAddOptionType(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	_ = r.ParseForm()
	name := strings.TrimSpace(r.FormValue("name"))
	values := splitCSVValues(r.FormValue("values"))
	def := strings.TrimSpace(r.FormValue("default"))
	if err := handler.catalogSrv.AddOptionType(r.Context(), id, name, values, def); err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Option type added", "info")
	}
	http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
}

// AdminUpdateOptionType renames an option type and/or replaces its values.
func (handler httpHandler) AdminUpdateOptionType(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	_ = r.ParseForm()
	currentName := strings.TrimSpace(r.FormValue("current_name"))
	newName := strings.TrimSpace(r.FormValue("new_name"))
	values := splitCSVValues(r.FormValue("values"))
	if err := handler.catalogSrv.UpdateOptionType(r.Context(), id, currentName, newName, values); err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Option type updated", "info")
	}
	http.Redirect(w, r, "/admin/products/"+id+"/edit", http.StatusSeeOther)
}

// AdminDeleteOptionType removes an option type from a product.
func (handler httpHandler) AdminDeleteOptionType(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	id := mux.Vars(r)["id"]
	_ = r.ParseForm()
	name := strings.TrimSpace(r.FormValue("name"))
	if err := handler.catalogSrv.DeleteOptionType(r.Context(), id, name); err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Option type deleted", "info")
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
