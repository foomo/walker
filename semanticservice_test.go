package walker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/foomo/walker/config"
	"github.com/foomo/walker/semanticscrape"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const semanticPage = `<html><body><main><article aria-label="Teaser"><h2>Hello</h2><p>text</p></article></main></body></html>`

func TestGetSemanticDocumentRendered(t *testing.T) {
	result := (&service{}).GetSemanticDocument("http://localhost/page", semanticPage, false)
	assert.Empty(t, result.Error)
	assert.Equal(t, SchemaSourceRendered, result.Source)
	assert.Equal(t, "http://localhost/page", result.URL)
	assert.Zero(t, result.StatusCode)
	require.NotNil(t, result.Document)
	require.NotNil(t, result.Document.Main())
	main := result.Document.Main().Children
	require.Len(t, main, 1)
	assert.Equal(t, semanticscrape.ElementTypeArticle, main[0].ElementType)
	assert.Equal(t, "Teaser", main[0].Label)
	assert.Equal(t, "Hello", main[0].Children[0].Text)

	// html without a url is fine, the url only names the page
	assert.Empty(t, (&service{}).GetSemanticDocument("", semanticPage, false).Error)

	// content on request
	withContent := (&service{}).GetSemanticDocument("", semanticPage, true)
	require.NotNil(t, withContent.Document)
	article := withContent.Document.Main().Children[0]
	require.Len(t, article.Children, 2)
	assert.Equal(t, semanticscrape.ElementTypeText, article.Children[1].ElementType)
	assert.Equal(t, "text", article.Children[1].Text)
}

func TestGetSemanticDocumentOnTheWire(t *testing.T) {
	encode := func(html string) string {
		result := (&service{}).GetSemanticDocument("", html, false)
		require.Empty(t, result.Error)
		b, err := json.Marshal(result.Document)
		require.NoError(t, err)
		return string(b)
	}
	assert.JSONEq(t, `{"Lang":"","Title":"","Description":"","Body":[]}`, encode(`<body><p>no main</p></body>`))
	assert.JSONEq(t, `{"Lang":"en","Title":"T","Description":"","Body":[{"ElementType":"main","Selector":"html > body > main","Text":"","Label":"","Links":null,"Src":"","Children":null}]}`,
		encode(`<html lang="en"><title>T</title><main><p>no structure</p></main></html>`))
}

func TestGetSemanticDocumentInvalidURL(t *testing.T) {
	for targetURL, expected := range map[string]string{
		"":                    "neither a url nor html",
		"file:///etc/passwd":  `not an http(s) url: "file:///etc/passwd"`,
		"/relative/path":      `not an http(s) url: "/relative/path"`,
		"ftp://example.com/x": `not an http(s) url: "ftp://example.com/x"`,
	} {
		result := (&service{}).GetSemanticDocument(targetURL, "", false)
		assert.Equal(t, SchemaSourceServer, result.Source, targetURL)
		assert.Equal(t, expected, result.Error, targetURL)
		assert.Nil(t, result.Document, targetURL)
	}
}

func TestGetSemanticDocumentServer(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-agent", r.UserAgent())
		_, _ = w.Write([]byte(semanticPage))
	}))
	defer page.Close()
	s := &service{conf: &config.Config{Agent: "test-agent"}}
	result := s.GetSemanticDocument(page.URL+"/page", "", false)
	assert.Empty(t, result.Error)
	assert.Equal(t, SchemaSourceServer, result.Source)
	assert.Equal(t, http.StatusOK, result.StatusCode)
	require.NotNil(t, result.Document.Main())
	assert.Equal(t, "Teaser", result.Document.Main().Children[0].Label)
}

func TestGetSemanticDocumentServerErrorPage(t *testing.T) {
	page := httptest.NewServer(http.NotFoundHandler())
	defer page.Close()
	result := (&service{}).GetSemanticDocument(page.URL+"/missing", "", false)
	assert.Equal(t, http.StatusNotFound, result.StatusCode)
	assert.Equal(t, "the page returned 404 Not Found - nothing extracted", result.Error)
	assert.Nil(t, result.Document)
}
