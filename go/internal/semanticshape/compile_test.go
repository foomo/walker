package semanticshape

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeRoot writes a schema root from file name -> content
func writeRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	return dir
}

func body(content string) string {
	return "<html><head></head><body>" + content + "</body></html>"
}

func TestLint(t *testing.T) {
	teaser := `<val:component marker="teaser" selector=".teaser"><h3/></val:component>`
	for name, tc := range map[string]struct {
		files map[string]string
		want  string
	}{
		"unknown rule":                  {map[string]string{"default.shape.html": body(`<h1 val:foo=1/>`)}, `default.shape.html:1: unknown rule "foo" on <h1>`},
		"forgotten prefix":              {map[string]string{"default.shape.html": body(`<h1 score=1/>`)}, `attribute "score" looks like a rule, did you mean "val:score"?`},
		"v1 typo val=optional":          {map[string]string{"default.shape.html": body(`<h1 val=optional/>`)}, `attribute "val" looks like a rule`},
		"prefix on val element":         {map[string]string{"default.shape.html": body(`<val:selector for="h1" val:count=1/>`)}, `<val:selector> takes its rules without prefix: "count" instead of "val:count"`},
		"duplicate attribute":           {map[string]string{"default.shape.html": body(`<h1 val:score=1 val:score=2/>`)}, `duplicate attribute "val:score"`},
		"min greater max":               {map[string]string{"default.shape.html": body(`<h1 val:min=3 val:max=2/>`)}, "min 3 is greater than max 2"},
		"optional and min":              {map[string]string{"default.shape.html": body(`<h1 val:optional val:min=1/>`)}, "optional together with min or count"},
		"unknown element":               {map[string]string{"default.shape.html": body(`<val:foo/>`)}, "unknown element <val:foo>"},
		"unclosed element":              {map[string]string{"default.shape.html": "<html><body><main></body></html>"}, "<main> is not closed"},
		"missing include":               {map[string]string{"default.shape.html": body(`<val:include path="/nope"/>`)}, "val:include: /nope not found"},
		"include with children":         {map[string]string{"default.shape.html": body(`<val:include path="/a"><h1/></val:include>`), "a.shape.html": "<h2/>"}, "<val:include> must not have children"},
		"include cycle":                 {map[string]string{"default.shape.html": body(`<val:include path="/a"/>`), "a.shape.html": `<val:include path="b"/>`, "b.shape.html": `<val:include path="/a"/>`}, "cycle: /default -> /a -> /b -> /a"},
		"comp of a multi root file":     {map[string]string{"default.shape.html": body(`<val:comp path="/two"/>`), "two.shape.html": "<h1/><h2/>"}, "/two can not be instantiated, it has 2 top level elements"},
		"marker override":               {map[string]string{"default.shape.html": body(`<main><val:comp path="/teaser" val:marker="x"/></main>`), "teaser.shape.html": teaser}, "marker of a component can not be overridden"},
		"literal on val root":           {map[string]string{"default.shape.html": body(`<main><val:comp path="/teaser" class="x"/></main>`), "teaser.shape.html": teaser}, `literal attribute "class" can not be set on <val:component>`},
		"duplicate marker":              {map[string]string{"default.shape.html": body(""), "a.shape.html": teaser, "b.shape.html": teaser}, `marker "teaser" is already used by /a`},
		"unbounded maximum":             {map[string]string{"default.shape.html": body(`<p val:min=1 val:score=1/>`)}, "the maximum score is unbounded, add score-max"},
		"unbounded score-per":           {map[string]string{"default.shape.html": body(`<p val:score=1 val:score-per="words:10"/>`)}, "the maximum score is unbounded"},
		"component twice in context":    {map[string]string{"default.shape.html": body(`<main><val:choice><val:comp path="/teaser"/></val:choice><val:comp path="/teaser"/></main>`), "teaser.shape.html": teaser}, "/teaser is used more than once in this context"},
		"element in choice":             {map[string]string{"default.shape.html": body(`<main><val:choice><h1/></val:choice></main>`)}, "<val:choice> may only contain val:comp of components"},
		"settings outside html":         {map[string]string{"default.shape.html": body(`<val:settings exclude=".x"/>`)}, "<val:settings> has to be a direct child of <html>"},
		"unknown attribute on wildcard": {map[string]string{"default.shape.html": body(`<val:wildcard for="div" deep/>`)}, `unknown attribute "deep" on <val:wildcard>`},
		"invalid regex":                 {map[string]string{"default.shape.html": body(`<a val:attr-href="regex:(["/>`)}, "attr-href: invalid regex"},
		"every outside val:prop":        {map[string]string{"default.shape.html": body(`<h1 val:every/>`)}, `unknown rule "every" on <h1>`},
		"unknown attribute rule":        {map[string]string{"default.shape.html": body(`<a val:attr-href="absolute"/>`)}, `attr-href: unknown rule "absolute"`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadRoot(writeRoot(t, tc.files))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestLoadDemoRoot(t *testing.T) {
	root := loadDemoRoot(t)
	assert.Equal(t, []string{"/components/category-tile", "/components/feature-section", "/components/product-tile"}, func() (paths []string) {
		for _, c := range root.componentList {
			paths = append(paths, c.path)
		}
		return paths
	}())
	assert.Equal(t, 117, root.schemas["/groups/index"].max)
	assert.Equal(t, 182, root.schemas["/groups/category"].max)
	assert.Equal(t, 145, root.schemas["/groups/product"].max)
	assert.Equal(t, 44, root.schemas["/default"].max)
}

func TestResolvePath(t *testing.T) {
	assert.Equal(t, "/partials/head", resolvePath("/groups/content/landing", "/partials/head"))
	assert.Equal(t, "/partials/head", resolvePath("/groups/content/landing", "../../partials/head"))
	assert.Equal(t, "/groups/content/teaser", resolvePath("/groups/content/landing", "teaser.shape.html"))
}

func TestLoadRootShapeFilesOnly(t *testing.T) {
	// plain .html next to shape files is not part of the root - not even a broken one
	root, err := LoadRoot(writeRoot(t, map[string]string{"default.shape.html": body(""), "README.html": "<p>"}))
	require.NoError(t, err)
	assert.True(t, root.HasDefault())

	// a root from before *.shape.html says how to rename it
	_, err = LoadRoot(writeRoot(t, map[string]string{"default.html": body(""), "groups/product.html": body("")}))
	var lintErrs LintErrors
	require.ErrorAs(t, err, &lintErrs)
	require.Len(t, lintErrs, 1)
	assert.Equal(t, "default.html", lintErrs[0].File)
	assert.Contains(t, lintErrs[0].Message, "2 .html files")
	assert.Contains(t, lintErrs[0].Message, "rename e.g. default.html to default.shape.html")
}

func TestRootVersion(t *testing.T) {
	dir := writeRoot(t, map[string]string{"default.shape.html": body(""), "partials/a.shape.html": "<h1/>", "notes.txt": "x"})
	v1, err := RootVersion(dir)
	require.NoError(t, err)
	v2, _ := RootVersion(dir)
	assert.Equal(t, v1, v2, "nothing changed")

	// other files do not count
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("changed"), 0o644))
	v3, _ := RootVersion(dir)
	assert.Equal(t, v1, v3)

	// an edit, a new file and a removed one do
	require.NoError(t, os.WriteFile(filepath.Join(dir, "partials", "a.shape.html"), []byte("<h1 val:score=1/>"), 0o644))
	v4, _ := RootVersion(dir)
	assert.NotEqual(t, v1, v4)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "partials", "b.shape.html"), []byte("<h2/>"), 0o644))
	v5, _ := RootVersion(dir)
	assert.NotEqual(t, v4, v5)
	require.NoError(t, os.Remove(filepath.Join(dir, "partials", "b.shape.html")))
	v6, _ := RootVersion(dir)
	assert.NotEqual(t, v5, v6)

	_, err = RootVersion(filepath.Join(dir, "nope"))
	assert.Error(t, err)
}
