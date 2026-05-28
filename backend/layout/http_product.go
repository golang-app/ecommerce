package layout

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	pcapp "github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/gorilla/mux"
)

// AllProducts renders the filterable product grid fragment (no base layout).
// The query string drives the filter: `category` selects the scope, while each
// filterable attribute type contributes either `<typeID>_min`/`<typeID>_max`
// (numeric) or a repeated `<typeID>` checkbox group (enum).
func (handler httpHandler) AllProducts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	category := q.Get("category")
	search := strings.TrimSpace(q.Get("q"))

	query := pcapp.ProductQuery{
		CategorySlug:   category,
		NumericRanges:  map[string]pcapp.Range{},
		EnumSelections: map[string][]string{},
		Search:         search,
	}

	// Use the facets for this scope to know which params are numeric vs enum.
	facets, err := handler.catalogSrv.Facets(ctx, category)
	if err != nil {
		handler.logger.WithError(err).WithField("category", category).Warn("cannot get facets")
		facets = nil
	}

	for _, f := range facets {
		typeID := f.Type.ID()
		switch {
		case f.Type.IsNumeric():
			minStr := q.Get(typeID + "_min")
			maxStr := q.Get(typeID + "_max")
			if minStr == "" && maxStr == "" {
				continue
			}
			rng := pcapp.Range{}
			if minStr != "" {
				if v, errParse := strconv.ParseFloat(minStr, 64); errParse == nil {
					rng.Min = &v
				}
			}
			if maxStr != "" {
				if v, errParse := strconv.ParseFloat(maxStr, 64); errParse == nil {
					rng.Max = &v
				}
			}
			if rng.Min == nil && rng.Max == nil {
				continue
			}
			query.NumericRanges[typeID] = rng
		case f.Type.IsEnum():
			if vals := q[typeID]; len(vals) > 0 {
				query.EnumSelections[typeID] = vals
			}
		}
	}

	products, err := handler.catalogSrv.List(ctx, query)
	if err != nil {
		https.InternalError(w, "internal-error", "cannot get list of all products")
		handler.logger.WithError(err).Error("cannot get list of all products")
		return
	}

	resp := map[string]any{
		"Products": products,
		"Search":   search,
	}

	files := []string{
		"./layout/tmpl/productCatalog/allProducts.gohtml",
	}

	var ts = template.Must(template.New("").Funcs(template.FuncMap{
		"html": func(value interface{}) template.HTML {
			return template.HTML(fmt.Sprint(value))
		},
		"add": func(a, b string) float64 {
			return 666
		},
	}).ParseFiles(files...))
	err = ts.ExecuteTemplate(w, "allProducts.gohtml", resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (handler httpHandler) Product(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["productID"]
	product, err := handler.catalogSrv.Find(r.Context(), id)

	session, _ := store.Get(r, "ecommerce")

	if err != nil {
		if errors.Is(err, domain.ErrProductNotFound) {
			session.AddFlash("Product does not exists", "error")
			_ = session.Save(r, w)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		session.AddFlash("cannot get list of all products", "error")
		_ = session.Save(r, w)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Resolve the initially-selected variant: the combination formed by the
	// first value of each option type (which is what the selects default to).
	selected := map[string]string{}
	for _, ot := range product.OptionTypes() {
		if len(ot.Values()) > 0 {
			selected[ot.Name()] = ot.Values()[0]
		}
	}
	variant := product.DefaultVariant()
	if product.HasOptions() {
		if v, ok := product.ResolveVariant(selected); ok {
			variant = v
		}
	}

	handler.renderTemplate(w, r, "productCatalog/show", map[string]any{
		"Product": product,
		"Variant": variant,
	})
}

// ProductVariant resolves the option selection (query params) to a variant
// and returns the variant box partial (price + add-to-cart). Driven by the
// option selects via HTMX.
func (handler httpHandler) ProductVariant(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["productID"]
	product, err := handler.catalogSrv.Find(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	selected := map[string]string{}
	for _, ot := range product.OptionTypes() {
		selected[ot.Name()] = r.URL.Query().Get(ot.Name())
	}
	variant, _ := product.ResolveVariant(selected)

	ts := template.Must(template.New("").ParseGlob("./layout/tmpl/partials/*.gohtml"))
	if err := ts.ExecuteTemplate(w, "variant-response", map[string]any{
		"Variant":     variant,
		"ProductName": product.Name(),
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
