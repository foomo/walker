package demoshop

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

//go:embed catalogue templates static
var files embed.FS

// GroupHeader tells walker the page group of a page: index, category or product
const GroupHeader = "x-next-js-app"

const siteName = "Semantic Shape Demo Shop"

// Run serves the demo shop on addr. With the sources at hand - go run in the
// repository - templates, css and catalogue are read from disk on every
// request, so edits show up with a reload. A binary without them, or
// embedded, serves the files compiled into it.
func Run(addr string, embedded bool) error {
	fsys, reload := SourceFiles()
	if embedded || !reload {
		fsys, reload = EmbeddedFiles(), false
	}
	handler, err := NewHandler(fsys, reload)
	if err != nil {
		return err
	}
	if reload {
		slog.Info("serving the sources, edits show up with a reload")
	}
	return http.ListenAndServe(addr, handler)
}

// EmbeddedFiles are templates, css and catalogue as compiled into the binary
func EmbeddedFiles() fs.FS {
	return files
}

// SourceFiles are templates, css and catalogue in the source directory -
// false if the binary was built somewhere else
func SourceFiles() (fs.FS, bool) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return nil, false
	}
	dir := filepath.Dir(file)
	if info, err := os.Stat(filepath.Join(dir, "templates")); err != nil || !info.IsDir() {
		return nil, false
	}
	return os.DirFS(dir), true
}

// SchemaDir is the shape root of the shop, in the source directory -
// the golden tests of semanticshape/SPEC.md and the walker tests validate against it
func SchemaDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "schema")
}

// NewHandler serves the shop from fsys - with reload, catalogue and
// templates are read again for every request:
//
//	/                     index - explains the demo
//	/category/<id>        category with its products
//	/product/<id>         product
//	/static/…, /images/…  css and product images
func NewHandler(fsys fs.FS, reload bool) (http.Handler, error) {
	s, err := newSite(fsys)
	if err != nil {
		return nil, err
	}
	if !reload {
		return s.routes(), nil
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// a site of its own per request - nothing is shared between requests
		s, err := newSite(fsys)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.routes().ServeHTTP(w, r)
	}), nil
}

type site struct {
	fsys      fs.FS
	directory *Directory
	templates *template.Template
}

func newSite(fsys fs.FS) (*site, error) {
	directory, err := Load(fsys)
	if err != nil {
		return nil, err
	}
	templates, err := template.New("").Funcs(template.FuncMap{
		"price":         formatPrice,
		"priceAmount":   priceAmount,
		"categoryImage": categoryImage,
		"add":           func(a, b int) int { return a + b },
	}).ParseFS(fsys, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &site{fsys: fsys, directory: directory, templates: templates}, nil
}

func (s *site) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /category/{id}", s.category)
	mux.HandleFunc("GET /product/{id}", s.product)
	mux.Handle("GET /static/", http.FileServerFS(s.fsys))
	mux.HandleFunc("GET /images/{category}/{file}", s.image)
	mux.HandleFunc("/", s.notFound)
	return mux
}

type crumb struct {
	Name string
	URL  string
}

// page is what the templates render - head, header and footer come from it
type page struct {
	Title       string
	Description string
	Canonical   string
	OGType      string
	OGImage     string
	SiteName    string
	Current     string
	Breadcrumbs []crumb
	LinkedData  []template.JS
	Directory   *Directory
	Category    *Category
	Product     *Product
}

func (s *site) render(w http.ResponseWriter, r *http.Request, status int, group, name string, p *page) {
	p.SiteName = siteName
	p.Directory = s.directory
	if group != "" {
		w.Header().Set(GroupHeader, group)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := s.templates.ExecuteTemplate(w, name, p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *site) index(w http.ResponseWriter, r *http.Request) {
	base := baseURL(r)
	p := &page{
		Title:       siteName + " – Robots and Cars",
		Description: "A small shop for funny robots and unusual cars, built to demonstrate how semantic shapes validate and scores the SEO of every page group.",
		Canonical:   base + "/",
		OGType:      "website",
		OGImage:     base + categorySocialImage(s.directory.Categories[0]),
	}
	p.LinkedData = append(p.LinkedData, linkedData(map[string]any{
		"@context": "https://schema.org",
		"@type":    "WebSite",
		"name":     siteName,
		"url":      base + "/",
	}))
	s.render(w, r, http.StatusOK, "index", "index", p)
}

func (s *site) category(w http.ResponseWriter, r *http.Request) {
	category := s.directory.Category(r.PathValue("id"))
	if category == nil {
		s.notFound(w, r)
		return
	}
	base := baseURL(r)
	p := &page{
		Title:       category.Name + " | " + siteName,
		Description: category.Description,
		Canonical:   base + "/category/" + category.ID,
		OGType:      "website",
		OGImage:     base + categorySocialImage(*category),
		Current:     category.ID,
		Category:    category,
		Breadcrumbs: []crumb{{"Home", "/"}, {category.Name, "/category/" + category.ID}},
	}
	// a host carousel: every list item is a complete product with its own page
	items := []map[string]any{}
	for i := range category.Products {
		product := &category.Products[i]
		items = append(items, map[string]any{
			"@type":    "ListItem",
			"position": i + 1,
			"item":     productLD(base, product, category, false),
		})
	}
	p.LinkedData = append(p.LinkedData,
		breadcrumbList(base, p.Breadcrumbs),
		linkedData(map[string]any{
			"@context":        "https://schema.org",
			"@type":           "ItemList",
			"name":            category.Name,
			"itemListElement": items,
		}),
	)
	s.render(w, r, http.StatusOK, "category", "category", p)
}

func (s *site) product(w http.ResponseWriter, r *http.Request) {
	product, category := s.directory.Product(r.PathValue("id"))
	if product == nil {
		s.notFound(w, r)
		return
	}
	base := baseURL(r)
	canonical := base + "/product/" + product.ID
	p := &page{
		Title:       product.Name + " – " + category.Name + " | Demo Shop",
		Description: product.Description,
		Canonical:   canonical,
		OGType:      "product",
		OGImage:     base + product.SocialImage,
		Current:     category.ID,
		Category:    category,
		Product:     product,
		Breadcrumbs: []crumb{{"Home", "/"}, {category.Name, "/category/" + category.ID}, {product.Name, "/product/" + product.ID}},
	}
	ld := productLD(base, product, category, true)
	ld["@context"] = "https://schema.org"
	p.LinkedData = append(p.LinkedData, linkedData(ld), breadcrumbList(base, p.Breadcrumbs))
	s.render(w, r, http.StatusOK, "product", "product", p)
}

// productLD is the Product structured data - with everything merchant
// listings want: brand, condition, shipping and the return policy. A list
// item of a carousel gets the short form.
func productLD(base string, product *Product, category *Category, full bool) map[string]any {
	url := base + "/product/" + product.ID
	offer := map[string]any{
		"@type":         "Offer",
		"url":           url,
		"price":         priceAmount(product),
		"priceCurrency": product.Currency,
		"availability":  "https://schema.org/InStock",
	}
	ld := map[string]any{
		"@type":  "Product",
		"name":   product.Name,
		"url":    url,
		"image":  base + product.Image,
		"offers": offer,
	}
	if !full {
		return ld
	}
	ld["description"] = product.Description
	ld["sku"] = product.ID
	ld["category"] = category.Name
	ld["brand"] = map[string]any{"@type": "Brand", "name": product.Brand}
	offer["itemCondition"] = "https://schema.org/NewCondition"
	offer["priceValidUntil"] = time.Now().AddDate(1, 0, 0).Format("2006-01-02")
	offer["shippingDetails"] = map[string]any{
		"@type": "OfferShippingDetails",
		"shippingRate": map[string]any{
			"@type":    "MonetaryAmount",
			"value":    "0.00",
			"currency": product.Currency,
		},
		"shippingDestination": map[string]any{
			"@type":          "DefinedRegion",
			"addressCountry": "DE",
		},
		"deliveryTime": map[string]any{
			"@type":        "ShippingDeliveryTime",
			"handlingTime": map[string]any{"@type": "QuantitativeValue", "minValue": 0, "maxValue": 1, "unitCode": "DAY"},
			"transitTime":  map[string]any{"@type": "QuantitativeValue", "minValue": 1, "maxValue": 3, "unitCode": "DAY"},
		},
	}
	offer["hasMerchantReturnPolicy"] = map[string]any{
		"@type":                "MerchantReturnPolicy",
		"applicableCountry":    "DE",
		"returnPolicyCategory": "https://schema.org/MerchantReturnFiniteReturnWindow",
		"merchantReturnDays":   30,
		"returnMethod":         "https://schema.org/ReturnByMail",
		"returnFees":           "https://schema.org/FreeReturn",
	}
	return ld
}

var imageTypes = map[string]string{".svg": "image/svg+xml", ".png": "image/png"}

// image serves the svg and png images of a product - only files of the catalogue
func (s *site) image(w http.ResponseWriter, r *http.Request) {
	category, file := r.PathValue("category"), r.PathValue("file")
	contentType, ok := imageTypes[path.Ext(file)]
	if s.directory.Category(category) == nil || !ok || strings.Contains(file, "..") {
		s.notFound(w, r)
		return
	}
	data, err := fs.ReadFile(s.fsys, path.Join(productsDir, category, file))
	if err != nil {
		s.notFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(data)
}

func (s *site) notFound(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusNotFound, "", "notfound", &page{
		Title:       "Not found | " + siteName,
		Description: "This page does not exist - maybe a robot took it for a joy ride in one of our cars.",
		Canonical:   baseURL(r) + r.URL.Path,
		OGType:      "website",
	})
}

// baseURL is the absolute url of the site, behind a proxy too
func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	host := r.Host
	if forwarded := r.Header.Get("X-Forwarded-Host"); forwarded != "" {
		host = forwarded
	}
	return scheme + "://" + host
}

func breadcrumbList(base string, crumbs []crumb) template.JS {
	items := []map[string]any{}
	for i, c := range crumbs {
		items = append(items, map[string]any{
			"@type":    "ListItem",
			"position": i + 1,
			"name":     c.Name,
			"item":     base + c.URL,
		})
	}
	return linkedData(map[string]any{
		"@context":        "https://schema.org",
		"@type":           "BreadcrumbList",
		"itemListElement": items,
	})
}

// linkedData is JSON for a script[type="application/ld+json"] - json.Marshal
// escapes <, > and &, so it can not end the script
func linkedData(v any) template.JS {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return template.JS("{}")
	}
	return template.JS(data)
}

var currencySymbols = map[string]string{"EUR": "€", "USD": "$", "CHF": "CHF"}

// asProduct lets the template functions take products and pointers to them
func asProduct(v any) Product {
	switch p := v.(type) {
	case *Product:
		return *p
	case Product:
		return p
	}
	return Product{}
}

func priceAmount(v any) string {
	p := asProduct(v)
	return fmt.Sprintf("%d.%02d", p.Price/100, p.Price%100)
}

func formatPrice(v any) string {
	p := asProduct(v)
	symbol, ok := currencySymbols[p.Currency]
	if !ok {
		symbol = p.Currency
	}
	return priceAmount(p) + " " + symbol
}

// categoryImage is the image of the first product of a category
func categoryImage(c Category) string {
	for _, p := range c.Products {
		if p.Image != "" {
			return p.Image
		}
	}
	return ""
}

// categorySocialImage is the png of the first product of a category, for OpenGraph
func categorySocialImage(c Category) string {
	for _, p := range c.Products {
		if p.SocialImage != "" {
			return p.SocialImage
		}
	}
	return ""
}
