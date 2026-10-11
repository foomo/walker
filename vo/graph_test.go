package vo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func graphTestResults() map[string]ScrapeResult {
	ms := time.Millisecond
	results := map[string]ScrapeResult{}
	add := func(u string, code int, group string, d time.Duration, links LinkList) {
		results[u] = ScrapeResult{TargetURL: u, Code: code, Group: group, Duration: d, NormalizedLinks: links}
	}
	// every page links to the home page - navigation
	nav := "https://example.com/"
	add(nav, 200, "home", 10*ms, LinkList{"https://example.com/shop": 1, "https://example.com/blog/a": 1})
	add("https://example.com/shop", 200, "category", 20*ms, LinkList{nav: 1, "https://example.com/shop/men/x": 2, "https://example.com/shop/women/y": 1})
	add("https://example.com/shop/men/x", 200, "product", 30*ms, LinkList{nav: 1, "https://example.com/shop/women/y": 1, "https://example.com/shop/men/x": 1})
	add("https://example.com/shop/women/y", 404, "product", 40*ms, LinkList{nav: 1, "https://example.com/blog/a": 3})
	add("https://example.com/blog/a", 200, "content", 50*ms, LinkList{nav: 1, "https://example.com/shop": 1})
	return results
}

func TestBuildGraphFolders(t *testing.T) {
	graph := BuildGraph(graphTestResults(), GraphOptions{Depth: 1})
	ids := []string{}
	for _, n := range graph.Nodes {
		ids = append(ids, n.ID)
	}
	assert.Equal(t, []string{"https://example.com", "https://example.com/blog", "https://example.com/shop"}, ids, "depth 1 - men and women are part of shop")
	shop := graph.Nodes[2]
	assert.Equal(t, "shop", shop.Label)
	assert.Equal(t, 3, shop.Pages, "the shop page, x and y")
	assert.Equal(t, 1, shop.Errors)
	assert.Equal(t, 404, shop.Code, "the worst status")
	assert.Equal(t, "product", shop.Group, "the most common group")
	assert.Equal(t, 40*time.Millisecond, shop.P90)
	assert.Equal(t, 2, shop.LinkedFrom, "the home page and blog/a - shop, x and y linking inside shop do not count")

	if assert.Len(t, graph.Global, 1) {
		assert.Equal(t, GlobalLink{URL: "https://example.com/", LinkedFrom: 4}, graph.Global[0], "linked from 4 of 5 pages")
	}
	// home -> shop 1, home -> blog 1, shop -> blog 3, blog -> shop 1 - links inside shop and to the home page are not edges
	assert.Equal(t, []GraphEdge{
		{From: 0, To: 1, Links: 1},
		{From: 0, To: 2, Links: 1},
		{From: 1, To: 2, Links: 1},
		{From: 2, To: 1, Links: 3},
	}, graph.Edges)
	assertEdgesValid(t, graph)
}

func TestBuildGraphFolderDepth(t *testing.T) {
	graph := BuildGraph(graphTestResults(), GraphOptions{Depth: 2})
	assert.Len(t, graph.Nodes, 5, "host, blog, shop, shop/men, shop/women")
	assertEdgesValid(t, graph)
}

func TestBuildFolderPages(t *testing.T) {
	results := graphTestResults()
	// depth 1: shop stands for the shop page and the pages below it
	pages := BuildFolderPages(results, GraphOptions{Depth: 1}, "https://example.com/shop", 0)
	ids := []string{}
	for _, p := range pages.Pages {
		ids = append(ids, p.ID)
	}
	assert.Equal(t, []string{"https://example.com/shop", "https://example.com/shop/men/x", "https://example.com/shop/women/y"}, ids)
	assert.Equal(t, 0, pages.Dropped)
	assert.Equal(t, 404, pages.Pages[2].Code)
	assert.Equal(t, "/shop/women/y", pages.Pages[2].Label)
	// shop -> x 2, shop -> y 1, x -> y 1 - x's self link and the links to the home page are no edges
	assert.Equal(t, []GraphEdge{{From: 0, To: 1, Links: 2}, {From: 0, To: 2, Links: 1}, {From: 1, To: 2, Links: 1}}, pages.Edges)

	// depth 2: shop has only its own page - men and women are folders of their own
	pages = BuildFolderPages(results, GraphOptions{Depth: 2}, "https://example.com/shop", 0)
	if assert.Len(t, pages.Pages, 1) {
		assert.Equal(t, "https://example.com/shop", pages.Pages[0].ID)
	}

	// the most linked pages are kept
	pages = BuildFolderPages(results, GraphOptions{Depth: 1}, "https://example.com/shop", 2)
	assert.Equal(t, 1, pages.Dropped)
	assert.Len(t, pages.Pages, 2)
	assertEdgesValidIn(t, len(pages.Pages), pages.Edges)

	assert.Empty(t, BuildFolderPages(results, GraphOptions{}, "https://example.com/unknown", 0).Pages)
}

func TestBuildGraphEmpty(t *testing.T) {
	graph := BuildGraph(nil, GraphOptions{})
	assert.NotNil(t, graph.Nodes)
	assert.NotNil(t, graph.Edges)
	assert.NotNil(t, graph.Global)
}

func assertEdgesValid(t *testing.T, graph Graph) {
	t.Helper()
	assertEdgesValidIn(t, len(graph.Nodes), graph.Edges)
}

func assertEdgesValidIn(t *testing.T, nodes int, edges []GraphEdge) {
	t.Helper()
	for _, e := range edges {
		assert.True(t, e.From >= 0 && e.From < nodes && e.To >= 0 && e.To < nodes, "edge %v out of range", e)
		assert.True(t, e.Links > 0)
	}
}

func TestBuildPageFolderLinks(t *testing.T) {
	results := graphTestResults()
	// depth 1: host 0, blog 1, shop 2 - y links to blog/a 3 times and to the home page (navigation)
	assert.Equal(t, []FolderLink{{Folder: 1, Links: 3}}, BuildPageFolderLinks(results, GraphOptions{Depth: 1}, "https://example.com/shop/women/y"))
	// the shop page links to x and y - both inside its own folder
	assert.Empty(t, BuildPageFolderLinks(results, GraphOptions{Depth: 1}, "https://example.com/shop"))
	// depth 2: x, y are folders of their own now - shop links to both
	assert.Equal(t, []FolderLink{{Folder: 3, Links: 2}, {Folder: 4, Links: 1}}, BuildPageFolderLinks(results, GraphOptions{Depth: 2}, "https://example.com/shop"))
	assert.NotNil(t, BuildPageFolderLinks(results, GraphOptions{}, "https://example.com/unknown"))
}
