package vo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBrokenLinks(t *testing.T) {
	results := map[string]ScrapeResult{
		"https://example.com/": {
			TargetURL:       "https://example.com/",
			Code:            200,
			NormalizedLinks: LinkList{"https://example.com/gone": 2, "https://example.com/ok": 1, "https://example.com/down": 1},
		},
		"https://example.com/ok": {
			TargetURL:       "https://example.com/ok",
			Code:            200,
			Group:           "content",
			NormalizedLinks: LinkList{"https://example.com/gone": 1},
		},
		"https://example.com/gone":   {TargetURL: "https://example.com/gone", Code: 404, Status: "404 Not Found"},
		"https://example.com/down":   {TargetURL: "https://example.com/down", Code: 0, Error: "timeout"},
		"https://example.com/orphan": {TargetURL: "https://example.com/orphan", Code: 500},
	}

	broken := BrokenLinks(results)
	if !assert.Len(t, broken, 3, "404, a failed request and a 500 - not the 200s") {
		return
	}
	gone := broken[0]
	assert.Equal(t, "https://example.com/gone", gone.TargetURL, "the most linked first")
	assert.Equal(t, 404, gone.Code)
	assert.Equal(t, 3, gone.Links, "two links from / and one from /ok")
	assert.Equal(t, []LinkSource{
		{TargetURL: "https://example.com/", Count: 2},
		{TargetURL: "https://example.com/ok", Group: "content", Count: 1},
	}, gone.LinkedFrom)

	assert.Equal(t, "https://example.com/down", broken[1].TargetURL)
	assert.Equal(t, "timeout", broken[1].Error)
	assert.Equal(t, "https://example.com/orphan", broken[2].TargetURL, "broken but not linked - still listed")
	assert.Empty(t, broken[2].LinkedFrom)

	assert.NotNil(t, BrokenLinks(nil), "an empty list, not null")
}
