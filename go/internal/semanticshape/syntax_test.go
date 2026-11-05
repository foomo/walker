package semanticshape

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/html"
)

func TestParseSyntax(t *testing.T) {
	doc, errs := parseSyntax("test.html", []byte(`<!-- comment -->
<html>
    <head>
        <title val:score=10 val:min-length=30/>
        <meta name="description"/>
    </head>
    <body>
        <val:selector for="h1" count=1/>
        <img val:optional>
        <h1 val:optional/>
    </body>
</html>
`))
	assert.Empty(t, errs)
	root := doc.children[0]
	assert.Equal(t, "html", root.name)
	head, body := root.children[0], root.children[1]
	assert.Equal(t, []string{"title", "meta"}, names(head.children), "self closing raw text tag must not swallow the head")
	assert.Equal(t, []string{"val:selector", "img", "h1"}, names(body.children))
	title := head.children[0]
	assert.Equal(t, 4, title.line)
	assert.Equal(t, []html.Attribute{{Key: "val:score", Val: "10"}, {Key: "val:min-length", Val: "30"}}, title.attrs, "the / of /> is not part of an unquoted value")
	assert.Equal(t, []html.Attribute{{Key: "for", Val: "h1"}, {Key: "count", Val: "1"}}, body.children[0].attrs)
	assert.Equal(t, 9, body.children[1].line)
}

func TestParseSyntaxErrors(t *testing.T) {
	_, errs := parseSyntax("test.html", []byte("<html>\n<body>\n<main>\n</body>\n</html>\n</p>"))
	assert.Equal(t, LintErrors{
		{File: "test.html", Line: 3, Message: "<main> is not closed"},
		{File: "test.html", Line: 6, Message: "</p> without an open <p>"},
	}, errs)
}

func names(nodes []*rawNode) (names []string) {
	for _, n := range nodes {
		names = append(names, n.name)
	}
	return names
}
