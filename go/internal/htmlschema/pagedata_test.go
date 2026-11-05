package htmlschema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractPageData(t *testing.T) {
	_, htmlBytes := renderDemo(t, "/product/robot-01")
	data, err := ExtractPageData(htmlBytes)
	require.NoError(t, err)

	assert.Equal(t, &HeadElement{Tag: "html", Attributes: []HeadAttribute{{"lang", "en"}}, Locator: "html"}, data.HTML)
	require.NotEmpty(t, data.Head)
	title := data.Head[2]
	assert.Equal(t, "title", title.Tag)
	assert.Contains(t, title.Text, "Clanky the Butlerbot")
	assert.Equal(t, "html > head > title", title.Locator)
	last := data.Head[len(data.Head)-1]
	assert.Equal(t, "script", last.Tag)
	assert.Equal(t, "JSON-LD, see below", last.Text)

	require.Len(t, data.LinkedData, 2)
	product, breadcrumbs := data.LinkedData[0], data.LinkedData[1]
	assert.Equal(t, last.Locator, breadcrumbs.Locator)
	assert.Equal(t, []string{"Product"}, product.Types)
	assert.Equal(t, []string{"BreadcrumbList"}, breadcrumbs.Types)
	assert.Empty(t, product.Error)
	assert.Contains(t, product.JSON, "\n  \"@context\"")

	// the locators are the ones of a trace
	_, trace, err := loadDemoRoot(t).Trace("product", htmlBytes)
	require.NoError(t, err)
	assert.Equal(t, []string{product.Locator}, traceNode(t, trace, "html/body/val:ld(Product)").Locators)
	assert.Equal(t, []string{title.Locator}, traceNode(t, trace, "html/head/title").Locators)
}

func TestExtractPageDataBrokenJSON(t *testing.T) {
	data, err := ExtractPageData([]byte(`<html><body><script type="application/ld+json">{"@type": </script></body></html>`))
	require.NoError(t, err)
	require.Len(t, data.LinkedData, 1)
	assert.NotEmpty(t, data.LinkedData[0].Error)
	assert.Equal(t, `{"@type":`, data.LinkedData[0].JSON)
	assert.Empty(t, data.Head)
}
