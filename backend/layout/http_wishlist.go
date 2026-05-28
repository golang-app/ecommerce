package layout

import (
	"errors"
	"html/template"
	"net/http"
	"path/filepath"

	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	pcdomain "github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	"github.com/gorilla/mux"
)

// wishlistEntry is the view-model the /account/wishlist page renders: one
// row per saved variant, with the catalogue data hydrated so the template
// can show a product card (thumbnail, name, price, link). The fields are
// exported so html/template can read them.
type wishlistEntry struct {
	VariantID   string
	ProductID   string
	ProductName string
	Thumbnail   string
	Price       pcdomain.Price
	InStock     bool
}

// WishlistToggle is the HTMX-driven endpoint behind the heart button on
// the product page. It flips the (customer, variant) bookmark and
// responds with the button in its new state — htmx swaps the original
// button element with the response (outerHTML) so the page reflects the
// change without a reload. Login-required: anonymous visitors are sent to
// the login page (the button itself is rendered as a "log in to save"
// hint for them, so this branch only ever fires after cookie expiry).
func (handler httpHandler) WishlistToggle(w http.ResponseWriter, r *http.Request) {
	cid, ok := handler.requireLogin(w, r)
	if !ok {
		return
	}
	variantID := mux.Vars(r)["variantID"]
	added, err := handler.wishlistSrv.Toggle(r.Context(), cid, variantID)
	if err != nil {
		handler.logger.WithError(err).Warn("wishlist toggle failed")
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	// HTMX-driven submits (the product-page heart button) want the new
	// button fragment in their outerHTML swap. Plain form submits (the
	// "remove" form on /account/wishlist) want to navigate back to where
	// they came from — typically the wishlist page itself, so the
	// just-removed row disappears.
	if r.Header.Get("HX-Request") == "true" {
		handler.renderWishlistButton(w, r, variantID, added, true)
		return
	}
	dest := r.Referer()
	if dest == "" {
		dest = "/account/wishlist"
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

// AccountWishlist renders the customer's wishlist page. Each item is
// hydrated through catalogSrv.FindVariant so the template can show a
// product card; variants whose product has been deleted are skipped (the
// table's FK cascade will catch up on the next admin write, this is
// belt-and-braces against an in-flight delete).
func (handler httpHandler) AccountWishlist(w http.ResponseWriter, r *http.Request) {
	cid, ok := handler.requireLogin(w, r)
	if !ok {
		return
	}
	items, err := handler.wishlistSrv.ListByCustomer(r.Context(), cid)
	if err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}

	entries := make([]wishlistEntry, 0, len(items))
	for _, it := range items {
		product, variant, ferr := handler.catalogSrv.FindVariant(r.Context(), it.VariantID())
		if ferr != nil {
			if errors.Is(ferr, pcdomain.ErrProductNotFound) {
				continue
			}
			handler.logger.WithError(ferr).WithField("variant_id", it.VariantID()).Warn("cannot hydrate wishlist item")
			continue
		}
		thumb := product.Thumbnail()
		if variant.Image() != "" {
			thumb = variant.Image()
		}
		entries = append(entries, wishlistEntry{
			VariantID:   variant.ID(),
			ProductID:   string(product.ID()),
			ProductName: product.Name(),
			Thumbnail:   thumb,
			Price:       variant.Price(),
			InStock:     variant.InStock(),
		})
	}

	handler.renderTemplate(w, r, "account/wishlist", map[string]any{
		"Active":  "wishlist",
		"Email":   cid,
		"Entries": entries,
	})
}

// renderWishlistButton emits the heart-button fragment for HTMX outerHTML
// swaps. The same fragment definition is invoked from the product-page
// template (via the "wishlist-button" partial); routing the toggle
// response through the same template keeps the two button states in sync.
//
// We parse the whole partials glob (rather than just the wishlist-button
// file) because the partials cross-reference each other; html/template
// requires every {{ funcName }} mentioned in any parsed template to be
// registered, so the `money` helper that variant-box.gohtml uses must
// be wired here even though only the wishlist button executes.
func (handler httpHandler) renderWishlistButton(w http.ResponseWriter, r *http.Request, variantID string, inWishlist bool, loggedIn bool) {
	files, _ := filepath.Glob("./layout/tmpl/partials/*.gohtml")
	ts := template.Must(template.New("").Funcs(template.FuncMap{
		"dict":  templateDict,
		"money": moneyFunc(handler.rates, handler.currentCurrency(r)),
	}).ParseFiles(files...))
	if err := ts.ExecuteTemplate(w, "wishlist-button", map[string]any{
		"VariantID":  variantID,
		"InWishlist": inWishlist,
		"LoggedIn":   loggedIn,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
