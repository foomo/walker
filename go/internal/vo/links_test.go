package vo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLinks(t *testing.T) {
	results := map[string]ScrapeResult{
		"https://example.com/": {
			TargetURL:       "https://example.com/",
			NormalizedLinks: LinkList{"https://example.com/": 1, "https://example.com/a": 2, "https://other.org/": 1},
		},
		"https://example.com/a": {
			TargetURL:       "https://example.com/a",
			Group:           "content",
			Structure:       Structure{Title: "A"},
			NormalizedLinks: LinkList{"https://example.com/": 1},
		},
		"https://example.com/orphan": {TargetURL: "https://example.com/orphan"},
	}
	links := Links(results)
	if !assert.Len(t, links, 3) {
		return
	}
	root, a, orphan := links[0], links[1], links[2]
	assert.Equal(t, "https://example.com/", root.TargetURL, "sorted by url")
	assert.Equal(t, 1, root.LinkedFrom, "the self link does not count")
	assert.Equal(t, 3, root.LinksTo, "every distinct link on the page, external ones too")
	assert.Equal(t, 1, a.LinkedFrom)
	assert.Equal(t, 2, a.Links)
	assert.Equal(t, "A", a.Title)
	assert.Equal(t, 0, orphan.LinkedFrom, "nothing links to the orphan")

	assert.Equal(t, []LinkSource{{TargetURL: "https://example.com/", Count: 2}}, LinkedFrom(results, "https://example.com/a", 0).Sources)
	assert.Equal(t, []LinkSource{{TargetURL: "https://example.com/a", Group: "content", Count: 1}}, LinkedFrom(results, "https://example.com/", 0).Sources, "without the self link")
	assert.NotNil(t, LinkedFrom(results, "https://example.com/orphan", 0).Sources, "an empty list, not null")
}

func TestLinkedFromLimit(t *testing.T) {
	results := map[string]ScrapeResult{}
	add := func(u, group string, count int) {
		results[u] = ScrapeResult{TargetURL: u, Group: group, NormalizedLinks: LinkList{"https://example.com/nav": count}}
	}
	add("https://example.com/a", "content", 1)
	add("https://example.com/b", "product", 3)
	add("https://example.com/c", "product", 1)
	add("https://example.com/d", "product", 2)

	page := LinkedFrom(results, "https://example.com/nav", 2)
	assert.Equal(t, 4, page.Total)
	assert.Equal(t, 7, page.Links)
	assert.Equal(t, []LinkGroup{{"product", 3}, {"content", 1}}, page.Groups, "the largest group first")
	assert.Equal(t, []LinkSource{
		{TargetURL: "https://example.com/b", Group: "product", Count: 3},
		{TargetURL: "https://example.com/d", Group: "product", Count: 2},
	}, page.Sources, "the most links first, cut at the limit")
}
