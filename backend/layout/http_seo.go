package layout

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

// requestBaseURL derives the absolute base URL (scheme + host) for the current
// request. It honors X-Forwarded-Proto first (typical for proxies/load
// balancers), then falls back to inspecting r.TLS, and finally defaults to
// http. The host comes from r.Host so it works for both real deployments and
// localhost.
func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// Robots serves /robots.txt — an allow-all crawl directive plus a pointer to
// the sitemap so search engines can find it without guessing.
func (handler httpHandler) Robots(w http.ResponseWriter, r *http.Request) {
	base := requestBaseURL(r)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	body := "User-agent: *\nAllow: /\n\nSitemap: " + base + "/sitemap.xml\n"
	if _, err := w.Write([]byte(body)); err != nil {
		handler.logger.WithError(err).Warn("cannot write robots.txt")
	}
}

// sitemapURL is one <url> entry in the sitemap. Built via encoding/xml so
// slugs/ids containing reserved characters are escaped automatically.
type sitemapURL struct {
	XMLName    xml.Name `xml:"url"`
	Loc        string   `xml:"loc"`
	ChangeFreq string   `xml:"changefreq"`
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

// Sitemap serves /sitemap.xml — the storefront's URLs that should be
// discoverable by crawlers: the landing page, the shop, every category, and
// every product. We omit account/cart/checkout/order pages on purpose: they
// are either per-user or transactional and have no value as crawl targets.
func (handler httpHandler) Sitemap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	base := requestBaseURL(r)

	categories, err := handler.catalogSrv.Categories(ctx)
	if err != nil {
		handler.logger.WithError(err).Warn("sitemap: cannot get categories")
		categories = nil
	}

	products, err := handler.catalogSrv.AllProducts(ctx)
	if err != nil {
		handler.logger.WithError(err).Warn("sitemap: cannot get products")
		products = nil
	}

	urls := []sitemapURL{
		{Loc: base + "/", ChangeFreq: "weekly"},
		{Loc: base + "/products", ChangeFreq: "weekly"},
	}
	for _, c := range categories {
		urls = append(urls, sitemapURL{
			Loc:        base + "/category/" + c.Slug(),
			ChangeFreq: "weekly",
		})
	}
	for _, p := range products {
		urls = append(urls, sitemapURL{
			Loc:        base + "/product/" + string(p.ID()),
			ChangeFreq: "weekly",
		})
	}

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	if _, err := fmt.Fprint(w, xml.Header); err != nil {
		handler.logger.WithError(err).Warn("cannot write sitemap header")
		return
	}

	set := sitemapURLSet{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  urls,
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(set); err != nil {
		handler.logger.WithError(err).Warn("cannot encode sitemap")
	}
}

// truncateForMeta trims a string to a meta-description-friendly length
// (~160 chars). It cuts at the previous word boundary when possible and
// appends an ellipsis. Whitespace runs are normalized so a description
// pulled from a multi-line product blurb still reads cleanly.
func truncateForMeta(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, ".,;:!?-") + "…"
}
