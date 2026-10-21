package htmlschema

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/foomo/walker/htmlschema/demo/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadDemoRoot(t *testing.T) *Root {
	t.Helper()
	_, filename, _, _ := runtime.Caller(0)
	root, err := LoadRoot(filepath.Join(filepath.Dir(filename), "demo", "schema"))
	require.NoError(t, err)
	return root
}

// renderDemo renders a page of the demo shop, without a port - the group
// comes from its response header, as walker reads it
func renderDemo(t *testing.T, path string) (group string, html []byte) {
	t.Helper()
	handler, err := server.NewHandler(server.EmbeddedFiles(), false)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "https://shop.example"+path, nil))
	return w.Header().Get(server.GroupHeader), w.Body.Bytes()
}

func validateDemo(t *testing.T, root *Root, path string) *Report {
	t.Helper()
	group, htmlBytes := renderDemo(t, path)
	report, err := root.Validate(group, htmlBytes)
	require.NoError(t, err)
	buf := &bytes.Buffer{}
	report.Print(buf)
	t.Log("\n" + buf.String())
	return report
}

type expectedFinding struct {
	severity Severity
	penalty  int
	rule     string
	path     string
	locator  string
	message  string
}

func assertFindings(t *testing.T, expected []expectedFinding, report *Report) {
	t.Helper()
	actual := make([]expectedFinding, len(report.Findings))
	for i, f := range report.Findings {
		actual[i] = expectedFinding{f.Severity, f.Penalty, f.Rule, f.Path, f.Locator, f.Message}
	}
	assert.Equal(t, expected, actual)
}

func scoreNode(t *testing.T, report *Report, path string) *ScoreNode {
	t.Helper()
	var find func(s *ScoreNode) *ScoreNode
	find = func(s *ScoreNode) *ScoreNode {
		if s.Path == path {
			return s
		}
		for _, c := range s.Children {
			if found := find(c); found != nil {
				return found
			}
		}
		return nil
	}
	s := find(report.Score)
	require.NotNil(t, s, "no score node "+path)
	return s
}

func assertScore(t *testing.T, report *Report, path string, points, max int) {
	t.Helper()
	s := scoreNode(t, report, path)
	assert.Equal(t, [2]int{points, max}, [2]int{s.Points, s.Max}, path)
}

func assertResult(t *testing.T, report *Report, schema string, points, max, penalty int) {
	t.Helper()
	assert.Equal(t, schema, report.Schema)
	assert.Equal(t, [3]int{points, max, penalty}, [3]int{report.Score.Points, report.Score.Max, report.Penalty})
}

// the worked examples of SPEC.md section 14

func TestExampleIndex(t *testing.T) {
	report := validateDemo(t, loadDemoRoot(t), "/")
	assertFindings(t, []expectedFinding{}, report)
	assertResult(t, report, "/groups/index", 89, 117, 0)
	assertScore(t, report, "html/head", 34, 34)
	assertScore(t, report, "html/body/val:ld(WebSite)", 5, 5)
	assertScore(t, report, "html/body/main/h1", 20, 20)
	assertScore(t, report, "html/body/main/p[class=lead]", 10, 10)
	assertScore(t, report, "html/body/main/section[aria-labelledby=categories]", 16, 42)
	assertScore(t, report, "html/body/main/section[aria-labelledby=categories]/val:choice", 14, 40)
	assertScore(t, report, "html/body/main/section[aria-labelledby=categories]/category-tile[2]", 7, 7)
	assertScore(t, report, "html/body/main/section[aria-labelledby=about]", 4, 6)
	assert.Equal(t, map[string]*ComponentSummary{
		"/components/category-tile": {Instances: 2, Points: 14, Max: 14},
	}, report.Components)
}

func TestExampleCategory(t *testing.T) {
	report := validateDemo(t, loadDemoRoot(t), "/category/cars")
	assertFindings(t, []expectedFinding{
		{SeverityWarn, 3, "ld-prop:max-length", "html/body/val:ld(ItemList)/itemListElement.item.name[4]", "html > head > script:nth-of-type(2)", "got 75, expected at most 60"},
		{SeverityWarn, 3, "max-length", "html/body/main/product-tile[4]/a/h2", "html > body > main > div > article:nth-of-type(4) > a > h2", "got 75, expected at most 60"},
	}, report)
	assertResult(t, report, "/groups/category", 130, 182, 6)
	assertScore(t, report, "html/head", 34, 34)
	assertScore(t, report, "html/body/val:ld(BreadcrumbList)", 5, 5)
	assertScore(t, report, "html/body/val:ld(ItemList)", 0, 10)
	assertScore(t, report, "html/body/main/nav[aria-label=Breadcrumb]", 3, 3)
	assertScore(t, report, "html/body/main/h1", 20, 20)
	assertScore(t, report, "html/body/main/p[class=lead]", 10, 10)
	assertScore(t, report, "html/body/main/val:choice", 58, 100)
	assertScore(t, report, "html/body/main/product-tile[3]", 6, 6)
	assertScore(t, report, "html/body/main/product-tile[4]", 4, 6)
	assert.Equal(t, map[string]*ComponentSummary{
		"/components/product-tile": {Instances: 10, Points: 58, Max: 60, Penalty: 3},
	}, report.Components)

	// the same schema on the robots: no long names, full points for the list
	robots := validateDemo(t, loadDemoRoot(t), "/category/robots")
	assert.Empty(t, robots.Findings)
	assertResult(t, robots, "/groups/category", 142, 182, 0)
	assertScore(t, robots, "html/body/val:ld(ItemList)", 10, 10)
}

func TestExampleProduct(t *testing.T) {
	report := validateDemo(t, loadDemoRoot(t), "/product/robot-01")
	assertFindings(t, []expectedFinding{}, report)
	assertResult(t, report, "/groups/product", 121, 145, 0)
	assertScore(t, report, "html/head", 36, 36)
	assertScore(t, report, "html/body/val:ld(Product)", 31, 31)
	assertScore(t, report, "html/body/val:ld(BreadcrumbList)", 5, 5)
	assertScore(t, report, "html/body/main/nav[aria-label=Breadcrumb]", 3, 3)
	assertScore(t, report, "html/body/main/article", 46, 70)
	assertScore(t, report, "html/body/main/article/figure", 5, 5)
	assertScore(t, report, "html/body/main/article/h1", 20, 20)
	assertScore(t, report, "html/body/main/article/p[class=price]", 5, 5)
	assertScore(t, report, "html/body/main/article/p[class=description]", 4, 10)
	assertScore(t, report, "html/body/main/article/val:choice", 12, 30)
	assertScore(t, report, "html/body/main/article/feature-section[1]/dl", 4, 8)
	assert.Equal(t, map[string]*ComponentSummary{
		"/components/feature-section": {Instances: 2, Points: 12, Max: 20},
	}, report.Components)
}

func TestExampleShortDescription(t *testing.T) {
	// Tiny Tim: a description of one word, everywhere it is used
	report := validateDemo(t, loadDemoRoot(t), "/product/robot-07")
	assertFindings(t, []expectedFinding{
		{SeverityError, 10, "attr:min-length", "html/head/meta[name=description]@content", "html > head > meta:nth-of-type(3)", "got 6, expected at least 70"},
		{SeverityError, 10, "attr:min-length", "html/head/meta[property=og:description]@content", "html > head > meta:nth-of-type(7)", "got 6, expected at least 70"},
		{SeverityError, 10, "ld-prop:min-length", "html/body/val:ld(Product)/description", "html > head > script:nth-of-type(1)", "got 6, expected at least 70"},
		{SeverityError, 10, "min-words", "html/body/main/article/p[class=description]", "html > body > main > article > div > div > p:nth-of-type(2)", "got 1, expected at least 12"},
	}, report)
	assertResult(t, report, "/groups/product", 85, 145, 40)
	assertScore(t, report, "html/head/meta[name=description]", 0, 10)
	assertScore(t, report, "html/body/val:ld(Product)", 11, 31)
	assertScore(t, report, "html/body/main/article/p[class=description]", 0, 10)
}

func TestExampleNoFeatures(t *testing.T) {
	// Lazybot Deluxe: no feature sections at all
	report := validateDemo(t, loadDemoRoot(t), "/product/robot-10")
	assertFindings(t, []expectedFinding{
		{SeverityError, 10, "occurrence", "html/body/main/article/val:choice", "html > body > main > article", "got 0, expected 1..6"},
	}, report)
	assertResult(t, report, "/groups/product", 109, 145, 10)
	assertScore(t, report, "html/body/main/article/val:choice", 0, 30)
	assert.Empty(t, report.Components)
}

func TestExampleLongName(t *testing.T) {
	// the station wagon: a name of 75 characters breaks every length limit
	report := validateDemo(t, loadDemoRoot(t), "/product/car-04")
	assertFindings(t, []expectedFinding{
		{SeverityError, 20, "max-length", "html/head/title", "html > head > title", "got 94, expected at most 60"},
		{SeverityError, 10, "attr:max-length", "html/head/meta[property=og:title]@content", "html > head > meta:nth-of-type(6)", "got 94, expected at most 90"},
		{SeverityError, 10, "ld-prop:max-length", "html/body/val:ld(Product)/name", "html > head > script:nth-of-type(1)", "got 75, expected at most 60"},
		{SeverityError, 50, "max-length", "html/body/main/article/h1", "html > body > main > article > div > div > h1", "got 75, expected at most 60"},
	}, report)
	assertResult(t, report, "/groups/product", 69, 145, 90)
	assertScore(t, report, "html/head/title", 0, 10)
	assertScore(t, report, "html/body/val:ld(Product)", 11, 31)
	assertScore(t, report, "html/body/main/article/h1", 0, 20)
}

func TestExampleCurrency(t *testing.T) {
	// the Crypto Cruiser: priced in DOGE
	report := validateDemo(t, loadDemoRoot(t), "/product/car-09")
	assertFindings(t, []expectedFinding{
		{SeverityError, 10, "attr:one-of", "html/head/meta[property=product:price:currency]@content", "html > head > meta:nth-of-type(11)", `"DOGE" is not one of EUR|USD|CHF`},
		{SeverityError, 10, "ld-prop:one-of", "html/body/val:ld(Product)/offers.priceCurrency", "html > head > script:nth-of-type(1)", `"DOGE" is not one of EUR|USD|CHF`},
		{SeverityError, 10, "ld-prop:one-of", "html/body/val:ld(Product)/offers.shippingDetails.shippingRate.currency", "html > head > script:nth-of-type(1)", `"DOGE" is not one of EUR|USD|CHF`},
	}, report)
	assertResult(t, report, "/groups/product", 101, 145, 30)
	assertScore(t, report, "html/body/val:ld(Product)", 11, 31)
}

func TestExampleFallback(t *testing.T) {
	// the 404 page sends no group, it is validated with default.html
	report := validateDemo(t, loadDemoRoot(t), "/no-such-page")
	assertFindings(t, []expectedFinding{}, report)
	assertResult(t, report, "/default", 41, 44, 0)
	assertScore(t, report, "html/head/meta[property=og:image]", 0, 3)
}
