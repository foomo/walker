package vo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedirects(t *testing.T) {
	results := map[string]ScrapeResult{
		"https://example.com/": {
			TargetURL:       "https://example.com/",
			Code:            200,
			NormalizedLinks: LinkList{"https://example.com/old": 2, "https://example.com/temp": 1},
		},
		"https://example.com/old": {
			TargetURL: "https://example.com/old",
			Code:      200,
			Group:     "content",
			Redirects: []Redirect{{Code: 301, URL: "https://example.com/older"}, {Code: 308, URL: "https://example.com/new"}},
		},
		"https://example.com/temp": {
			TargetURL: "https://example.com/temp",
			Code:      404,
			Redirects: []Redirect{{Code: 302, URL: "https://example.com/gone"}},
		},
		"https://example.com/direct": {TargetURL: "https://example.com/direct", Code: 200},
	}

	redirects := Redirects(results)
	if !assert.Len(t, redirects, 2, "only urls with redirects") {
		return
	}
	old := redirects[0]
	assert.Equal(t, "https://example.com/old", old.TargetURL, "the longest chain first")
	assert.Len(t, old.Hops, 2)
	assert.Equal(t, "https://example.com/new", old.FinalURL)
	assert.Equal(t, 200, old.FinalCode)
	assert.True(t, old.Permanent, "301 and 308")
	assert.Equal(t, 2, old.Links)
	assert.Equal(t, []LinkSource{{TargetURL: "https://example.com/", Count: 2}}, old.LinkedFrom)

	temp := redirects[1]
	assert.False(t, temp.Permanent, "302 is temporary")
	assert.Equal(t, 404, temp.FinalCode, "the chain ends broken")

	assert.NotNil(t, Redirects(nil), "an empty list, not null")
}
