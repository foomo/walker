package htmlschema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func traceNode(t *testing.T, root *TraceNode, path string) *TraceNode {
	t.Helper()
	var find func(n *TraceNode) *TraceNode
	find = func(n *TraceNode) *TraceNode {
		if n.Path == path {
			return n
		}
		for _, c := range n.Children {
			if found := find(c); found != nil {
				return found
			}
		}
		return nil
	}
	n := find(root)
	require.NotNil(t, n, "no trace node "+path)
	return n
}

func TestTraceProduct(t *testing.T) {
	_, htmlBytes := renderDemo(t, "/product/car-04")
	root := loadDemoRoot(t)
	report, trace, err := root.Trace("product", htmlBytes)
	require.NoError(t, err)

	// the trace agrees with the report
	assert.Equal(t, "page", trace.Kind)
	assert.Equal(t, [3]int{report.Score.Points, report.Score.Max, report.Penalty}, [3]int{trace.Points, trace.Max, trace.Penalty})
	assert.Equal(t, TraceStatusOK, trace.Status)

	h1 := traceNode(t, trace, "html/body/main/article/h1")
	assert.Equal(t, TraceStatusError, h1.Status)
	assert.Equal(t, []string{"html > body > main > article > div > div > h1"}, h1.Locators)
	assert.Equal(t, [3]int{0, 20, 50}, [3]int{h1.Points, h1.Max, h1.Penalty})
	assert.Len(t, h1.Findings, 1)

	section := traceNode(t, trace, "html/body/main/article/feature-section[2]")
	assert.Equal(t, "component", section.Kind)
	assert.Equal(t, "feature-section[2]", section.Label)
	assert.Equal(t, [3]int{6, 10, 0}, [3]int{section.Points, section.Max, section.Penalty})

	dd := traceNode(t, trace, "html/body/main/article/feature-section[1]/dl/dd")
	assert.Len(t, dd.Locators, 2)
	assert.Equal(t, []string{"match", "match"}, []string{dd.Children[0].Kind, dd.Children[1].Kind})
	assert.Equal(t, "dd[2]", dd.Children[1].Label)
	assert.Equal(t, [2]int{2, 4}, [2]int{dd.Points, dd.Max}, "1 point each, capped at 4")

	// h1 is found through the wrapper divs of the article, the article itself is not
	assert.Equal(t, "div", h1.Via)
	assert.Empty(t, traceNode(t, trace, "html/body/main/article").Via)

	choice := traceNode(t, trace, "html/body/main/article/val:choice")
	assert.Equal(t, "choice", choice.Kind)
	assert.Len(t, choice.Children, 2)
	assert.Len(t, choice.Locators, 2)
}

func TestTraceMissing(t *testing.T) {
	// Lazybot Deluxe has no feature sections - the choice points to where they are missing
	_, htmlBytes := renderDemo(t, "/product/robot-10")
	_, trace, err := loadDemoRoot(t).Trace("product", htmlBytes)
	require.NoError(t, err)

	choice := traceNode(t, trace, "html/body/main/article/val:choice")
	assert.Equal(t, TraceStatusError, choice.Status)
	assert.Equal(t, []string{"html > body > main > article"}, choice.Locators)
	assert.Equal(t, 10, trace.Penalty)
}

func TestTraceAbsent(t *testing.T) {
	// the 404 page has no og:image - optional, so no finding
	_, htmlBytes := renderDemo(t, "/no-such-page")
	_, trace, err := loadDemoRoot(t).Trace("", htmlBytes)
	require.NoError(t, err)

	ogImage := traceNode(t, trace, "html/head/meta[property=og:image]")
	assert.Equal(t, TraceStatusAbsent, ogImage.Status)
	assert.Equal(t, []string{"html > head"}, ogImage.Locators, "points to where it is missing")
	assert.False(t, ogImage.Found)
	assert.Empty(t, ogImage.Findings)
	assert.Contains(t, ogImage.Rules, "optional score=3")
}

func TestRootGroups(t *testing.T) {
	root := loadDemoRoot(t)
	assert.Equal(t, []string{"category", "index", "product"}, root.Groups())
	assert.True(t, root.HasDefault())
}

func TestTraceSurplus(t *testing.T) {
	root, err := LoadRoot(writeRoot(t, map[string]string{"default.html": body(`<div><a val:score=1/></div>`)}))
	require.NoError(t, err)
	_, trace, err := root.Trace("x", []byte(pageBody(`<div><a>first</a><a>second</a></div>`)))
	require.NoError(t, err)

	a := traceNode(t, trace, "html/body/div/a")
	assert.Empty(t, a.Findings, "the surplus finding belongs to the surplus node")
	assert.Equal(t, TraceStatusOK, a.Status)
	assert.Equal(t, 10, a.Penalty)
	assert.Equal(t, []string{"html > body > div > a:nth-of-type(1)", "html > body > div > a:nth-of-type(2)"}, a.Locators)
	require.Len(t, a.Children, 2)

	first, second := a.Children[0], a.Children[1]
	assert.Equal(t, [3]any{"a[1]", TraceStatusOK, 1}, [3]any{first.Label, first.Status, first.Points})
	assert.Empty(t, first.Findings)
	assert.Equal(t, [2]any{"a[2]", TraceStatusError}, [2]any{second.Label, second.Status})
	assert.Equal(t, []string{"html > body > div > a:nth-of-type(2)"}, second.Locators)
	require.Len(t, second.Findings, 1)
	assert.Equal(t, "got 2, expected exactly 1", second.Findings[0].Message)
}
