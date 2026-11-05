package semanticshape

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validateInline validates a page against a schema root given as file name -> content
func validateInline(t *testing.T, files map[string]string, page string) *Report {
	t.Helper()
	root, err := LoadRoot(writeRoot(t, files))
	require.NoError(t, err)
	report, err := root.Validate("test", []byte(page))
	require.NoError(t, err)
	return report
}

type finding struct {
	severity Severity
	penalty  int
	rule     string
	path     string
}

func findings(report *Report) (list []finding) {
	for _, f := range report.Findings {
		list = append(list, finding{f.Severity, f.Penalty, f.Rule, f.Path})
	}
	return list
}

func pageBody(content string) string {
	return "<html><head></head><body>" + content + "</body></html>"
}

func TestWildcard(t *testing.T) {
	schema := body(`<val:wildcard for="div, span">
		<main>
			<h1 val:score=1/>
			<nav><val:wildcard for=""><a val:score=1/></val:wildcard></nav>
		</main>
	</val:wildcard>`)
	page := pageBody(`<div><div><main><div><span><h1>x</h1></span></div><nav><div><a href="/">a</a></div></nav></main></div></div>`)
	report := validateInline(t, map[string]string{"default.shape.html": schema}, page)
	// h1 is found through the inherited wildcard, the a is not: for="" makes nav strict
	assert.Equal(t, []finding{{SeverityError, 10, "occurrence", "html/body/main/nav/a"}}, findings(report))
	assert.Equal(t, 1, report.Score.Points)

	strict := validateInline(t, map[string]string{"default.shape.html": body(`<main/>`)}, page)
	assert.Equal(t, []finding{{SeverityError, 10, "occurrence", "html/body/main"}}, findings(strict))
}

func TestOccurrencePenaltyScales(t *testing.T) {
	report := validateInline(t, map[string]string{"default.shape.html": body(`
		<h1 val:penalty=50/>
		<nav><a val:min=3/></nav>
		<i val:forbidden val:penalty=20 val:penalty-max=30/>
	`)}, pageBody(`<h1>a</h1><h1>b</h1><h1>c</h1><nav></nav><i>1</i><i>2</i>`))
	assert.Equal(t, []finding{
		{SeverityError, 50, "occurrence", "html/body/h1[2]"},
		{SeverityError, 50, "occurrence", "html/body/h1[3]"},
		{SeverityError, 30, "occurrence", "html/body/nav/a"},
		{SeverityError, 20, "occurrence", "html/body/i[1]"},
		{SeverityError, 10, "occurrence", "html/body/i[2]"},
	}, findings(report))
	assert.Equal(t, 160, report.Penalty)
}

func TestScore(t *testing.T) {
	report := validateInline(t, map[string]string{"default.shape.html": body(`
		<p val:min=1 val:score=2 val:score-per="words:5" val:score-max=5/>
		<h2 val:optional val:score=3 val:min-length=10/>
	`)}, pageBody(`<p>one two three four five six</p><p>one two three four five six seven eight nine ten</p><h2>short</h2>`))
	// p: 2×1 + 2×2 = 6, capped at 5. h2 fails its own rule and earns nothing
	assertScore(t, report, "html/body/p", 5, 5)
	assertScore(t, report, "html/body/h2", 0, 3)
	assert.Equal(t, []finding{{SeverityError, 10, "min-length", "html/body/h2"}}, findings(report))
}

func TestSeverityAndPenaltyDefaults(t *testing.T) {
	report := validateInline(t, map[string]string{"default.shape.html": `<html>
		<val:settings penalty-warn=7/>
		<head><title val:severity=warn val:min-length=10/></head>
		<body><a val:attr-href="required; severity:info"/></body>
	</html>`}, `<html><head><title>x</title></head><body><a>x</a></body></html>`)
	assert.Equal(t, []finding{
		{SeverityWarn, 7, "min-length", "html/head/title"},
		{SeverityInfo, 0, "attr:required", "html/body/a@href"},
	}, findings(report))
}

func TestExclusion(t *testing.T) {
	report := validateInline(t, map[string]string{"default.shape.html": `<html>
		<val:settings exclude=".mobile-only, [aria-hidden=true]"/>
		<head/>
		<body><val:wildcard for="div"><nav/></val:wildcard><val:selector for="p" count=1/></body>
	</html>`}, pageBody(`<div><nav></nav></div><div class="mobile-only"><nav></nav></div><div hidden><p>x</p></div><nav aria-hidden="true"></nav><p>y</p>`))
	assert.Empty(t, findings(report))
}

const teaser = `<val:component marker="teaser" selector="[class*='Teaser_root']"><h3 val:score=1/></val:component>`
const video = `<val:component marker="video" selector="[class*='VideoTeaser_root']"><video val:score=1/></val:component>`

func TestComponentAmbiguity(t *testing.T) {
	report := validateInline(t, map[string]string{
		"default.shape.html": body(`<main><val:choice max=5><val:comp path="/teaser"/><val:comp path="/video"/></val:choice></main>`),
		"teaser.shape.html":  teaser,
		"video.shape.html":   video,
	}, pageBody(`<main><div class="Teaser_root__1"><h3>x</h3></div><div class="VideoTeaser_root__2"><video></video></div><div data-component="video"><video></video></div></main>`))
	assert.Equal(t, []finding{{SeverityError, 10, "component-ambiguous", ""}}, findings(report))
	assert.Equal(t, 2, report.Score.Points, "the ambiguous instance is not validated")
	assert.Contains(t, report.Findings[0].Message, "/teaser, /video")
}

func TestComponentContexts(t *testing.T) {
	files := map[string]string{
		"default.shape.html":           body(`<main><val:sequence><val:comp path="/video"/><val:comp path="/components/teaser" val:optional/></val:sequence></main><aside val:open><val:comp path="/video" val:optional/></aside>`),
		"components/teaser.shape.html": teaser,
		"video.shape.html":             video,
	}
	report := validateInline(t, files, pageBody(`<main><div data-component="teaser"><h3>x</h3></div><div data-component="video"><video></video></div><div data-component="nope"></div></main><aside><div data-component="teaser"></div></aside>`))
	// a marker without a component schema is ignored. The aside is open: a
	// component it does not use is only info, and it is not validated
	assert.Equal(t, []finding{
		{SeverityError, 10, "component-order", "html/body/main/video"},
		{SeverityInfo, 0, "component-not-allowed", "html/body/aside/teaser"},
	}, findings(report))
	assert.Equal(t, &ComponentSummary{Instances: 1, Points: 1, Max: 1}, report.Components["/components/teaser"])
}

func TestSeverityCeiling(t *testing.T) {
	report := validateInline(t, map[string]string{
		"default.shape.html": body(`<main><val:comp path="/teaser" val:severity=warn/></main>`),
		"teaser.shape.html":  teaser,
	}, pageBody(`<main><div data-component="teaser"></div></main>`))
	// the missing h3 is an error in the component, capped to warn - its default penalty stays the error's
	assert.Equal(t, []finding{{SeverityWarn, 10, "occurrence", "html/body/main/teaser/h3"}}, findings(report))
	assert.Equal(t, "/teaser", report.Findings[0].Component)
}

func TestCompOverrides(t *testing.T) {
	report := validateInline(t, map[string]string{
		"default.shape.html":   body(`<val:comp path="/nav" aria-label="Main" val:score=20/><val:comp path="/teaser" val:optional/>`),
		"nav.shape.html":       `<nav aria-label="x" val:score=10><a val:min=1/></nav>`,
		"teaser.shape.html":    `<val:component marker="teaser" min=2><h3/></val:component>`,
		"unrelated.shape.html": `<h1/><h2/>`,
	}, pageBody(`<nav aria-label="x"></nav><nav aria-label="Main"><a href="/">a</a></nav>`))
	// the literal is overridden, the score too, and val:optional replaces min=2 as a group
	assert.Empty(t, findings(report))
	assert.Equal(t, 20, report.Score.Points)
}

func TestChoiceMax(t *testing.T) {
	root, err := LoadRoot(writeRoot(t, map[string]string{
		"default.shape.html": body(`<main><val:choice max=3><val:comp path="/a" val:min=1/><val:comp path="/b" val:max=5/></val:choice></main>`),
		"a.shape.html":       `<val:component marker="a"><h1 val:score=2/></val:component>`,
		"b.shape.html":       `<val:component marker="b"><h1 val:score=5/></val:component>`,
	}))
	require.NoError(t, err)
	// first the minimum: 1 × a = 2, then the most valuable: 2 × b = 10
	assert.Equal(t, 12, root.schemas["/default"].max)
}

func TestLD(t *testing.T) {
	report := validateInline(t, map[string]string{"default.shape.html": body(`
		<val:ld type="Product" score=10>
			<val:prop path="name" min-length=5 score=1/>
			<val:prop path="offers.priceCurrency" value="one-of:EUR|CHF"/>
			<val:prop path="sku" optional value="min-length:3"/>
		</val:ld>
		<val:ld type="FAQPage" optional/>
	`)}, `<html><head><script type="application/ld+json">{"@graph":[{"@type":["Product","Thing"],"name":"Shoe","offers":[{"priceCurrency":"USD"},{"priceCurrency":"EUR"}],"sku":"x"}]}</script></head><body></body></html>`)
	assert.Equal(t, []finding{
		{SeverityError, 10, "ld-prop:min-length", "html/body/val:ld(Product)/name"},
		{SeverityError, 10, "ld-prop:min-length", "html/body/val:ld(Product)/sku"},
	}, findings(report))
	// name fails, so the object earns nothing - an optional prop failing would not block it
	assertScore(t, report, "html/body/val:ld(Product)", 0, 11)
}

func TestUnknownMarkerIsIgnored(t *testing.T) {
	report := validateInline(t, map[string]string{
		"default.shape.html": body(`<main><val:wildcard for="div"><a val:min=2/></val:wildcard></main>`),
		"teaser.shape.html":  teaser,
	}, pageBody(`<main><div data-component="Link"><a href="/a">a</a></div><div data-component="Teaser_root__1"><a href="/b">b</a></div></main>`))
	// the Link marker has no schema - its content is matched like any other
	assert.Empty(t, findings(report))
}

func TestLDEvery(t *testing.T) {
	page := `<html><head><script type="application/ld+json">{"@type":"ItemList","itemListElement":[
		{"@type":"ListItem","item":{"@type":"Product","offers":{"priceCurrency":"EUR"}}},
		{"@type":"ListItem","item":{"@type":"Product","offers":{"priceCurrency":"DOGE"}}},
		{"@type":"ListItem","item":{"@type":"Product","offers":{}}}
	]}</script></head><body></body></html>`
	// one passing element is enough without every
	report := validateInline(t, map[string]string{"default.shape.html": body(`
		<val:ld type="ItemList" score=10>
			<val:prop path="itemListElement.item.offers.priceCurrency" value="one-of:EUR|CHF" score=1/>
		</val:ld>
	`)}, page)
	assert.Empty(t, findings(report))
	assertScore(t, report, "html/body/val:ld(ItemList)", 11, 11)

	// every element has to pass - and has to have the property
	report = validateInline(t, map[string]string{"default.shape.html": body(`
		<val:ld type="ItemList" score=10>
			<val:prop path="itemListElement.item.offers.priceCurrency" every value="one-of:EUR|CHF" score=1/>
		</val:ld>
	`)}, page)
	assert.Equal(t, []finding{
		{SeverityError, 10, "ld-prop:one-of", "html/body/val:ld(ItemList)/itemListElement.item.offers.priceCurrency[2]"},
		{SeverityError, 10, "ld-prop:required", "html/body/val:ld(ItemList)/itemListElement.item.offers.priceCurrency[3]"},
	}, findings(report))
	assertScore(t, report, "html/body/val:ld(ItemList)", 0, 11)

	// optional and every: a missing property is fine, a wrong one is not
	report = validateInline(t, map[string]string{"default.shape.html": body(`
		<val:ld type="ItemList">
			<val:prop path="itemListElement.item.offers.priceCurrency" every optional value="one-of:EUR|CHF"/>
		</val:ld>
	`)}, page)
	assert.Equal(t, []finding{
		{SeverityError, 10, "ld-prop:one-of", "html/body/val:ld(ItemList)/itemListElement.item.offers.priceCurrency[2]"},
	}, findings(report))
}
