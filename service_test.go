package walker

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/foomo/walker/vo"
	"github.com/stretchr/testify/assert"
)

func TestPaginate(t *testing.T) {
	results := make([]vo.ScrapeResult, 5)
	for i := range results {
		results[i].Code = i
	}
	for _, tc := range []struct {
		name           string
		page, pageSize int
		wantCodes      []int
		wantPages      int
	}{
		{"first page", 0, 2, []int{0, 1}, 3},
		{"last partial page", 2, 2, []int{4}, 3},
		{"beyond the last page", 7, 2, []int{}, 3},
		{"negative page", -1, 2, []int{0, 1}, 3},
		{"no page size", 3, 0, []int{0, 1, 2, 3, 4}, 1},
		{"exact fit", 0, 5, []int{0, 1, 2, 3, 4}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, numPages := paginate(results, tc.page, tc.pageSize)
			codes := []int{}
			for _, r := range page {
				codes = append(codes, r.Code)
			}
			assert.Equal(t, tc.wantCodes, codes)
			assert.Equal(t, tc.wantPages, numPages)
		})
	}
}

func TestStatusSummary(t *testing.T) {
	started := time.Date(2026, time.October, 10, 14, 0, 0, 0, time.UTC)
	summary := vo.Status{
		Results: map[string]vo.ScrapeResult{
			"a": {Code: 200}, "b": {Code: 200}, "c": {Code: 404},
		},
		Jobs:    map[string]bool{"d": false},
		Running: true,
		Started: started,
	}.Summary()
	assert.Equal(t, 3, summary.Pages)
	assert.Equal(t, 1, summary.JobsOpen)
	assert.Equal(t, map[int]int{200: 2, 404: 1}, summary.StatusCodes)
	if assert.NotNil(t, summary.Started) {
		assert.True(t, started.Equal(*summary.Started))
	}
	assert.Nil(t, summary.Completed, "zero timestamps must be nil - the swift client can not decode a null Date")
	assert.Nil(t, summary.NextCycle)
}

func TestServiceProxy(t *testing.T) {
	s := &service{walker: NewWalker()}
	defer s.walker.Stop()
	proxy := NewDefaultServiceGoTSRPCProxy(s)

	call := func(method, body string) string {
		rec := httptest.NewRecorder()
		proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/service/"+method, strings.NewReader(body)))
		assert.Equal(t, http.StatusOK, rec.Code, method)
		b, _ := io.ReadAll(rec.Body)
		return strings.TrimSpace(string(b))
	}
	// nothing walked yet - no complete and no current scrape
	assert.Equal(t, "[null,null]", call("GetScrapes", "[]"))
	// no complete scrape and a page size of 0 must neither fail nor panic
	assert.Contains(t, call("GetResults", `[{"Scrape":"complete"},0,0]`), "[")
}

func TestServiceGetResultsFolders(t *testing.T) {
	s := &service{walker: NewWalker()}
	defer s.walker.Stop()
	for _, scrape := range []string{ScrapeCurrent, ScrapeComplete} {
		_, results, _, folders := s.GetResults(Filters{Scrape: scrape}, 0, 10)
		assert.Empty(t, results, scrape)
		assert.Empty(t, folders, scrape)
	}
}

func TestGetFoldersIgnoresFilters(t *testing.T) {
	// folders are built before filtering - the tree always shows the whole scrape
	resultMap := map[string]vo.ScrapeResult{
		"https://example.com/shop/a": {TargetURL: "https://example.com/shop/a", Code: 200},
		"https://example.com/blog/b": {TargetURL: "https://example.com/blog/b", Code: 404},
	}
	folders := getFolders(resultMap)
	filter(resultMap, getFilterChain(Filters{Prefix: "https://example.com/shop", Status: []int{200}}, true))
	assert.Len(t, resultMap, 1, "the filter only affects the results")
	if assert.Len(t, folders, 1) {
		assert.Equal(t, 2, folders[0].Pages)
		assert.Len(t, folders[0].Folders, 2)
	}
}

func TestGroupFilter(t *testing.T) {
	resultMap := map[string]vo.ScrapeResult{
		"a": {TargetURL: "a", Group: "product"},
		"b": {TargetURL: "b", Group: "product"},
		"c": {TargetURL: "c", Group: "content"},
		"d": {TargetURL: "d"},
	}
	options := getFilterOptions(resultMap)
	assert.Equal(t, []GroupStats{{"", 1}, {"content", 1}, {"product", 2}}, options.Groups, "sorted, results without a group included")

	filter(resultMap, getFilterChain(Filters{Groups: []string{"content", ""}}, false))
	assert.Len(t, resultMap, 2)
	assert.Contains(t, resultMap, "c")
	assert.Contains(t, resultMap, "d", `"" selects results without a group`)
}

func TestFolderPrefixFilter(t *testing.T) {
	targetURLs := []string{
		"https://example.com/shop",
		"https://example.com/shop/",
		"https://example.com/shop/a",
		"https://example.com/shop?page=2",
		"https://example.com/shopping/b",
	}
	resultMap := func() map[string]vo.ScrapeResult {
		m := map[string]vo.ScrapeResult{}
		for _, u := range targetURLs {
			m[u] = vo.ScrapeResult{TargetURL: u}
		}
		return m
	}
	folders := getFolders(resultMap())
	assert.True(t, containsFolder(folders, "https://example.com/shop"))
	assert.True(t, containsFolder(folders, "https://example.com"), "the host is a folder too")
	assert.False(t, containsFolder(folders, "https://example.com/sho"))

	inFolder := resultMap()
	filter(inFolder, getFilterChain(Filters{Prefix: "https://example.com/shop"}, true))
	assert.Len(t, inFolder, 4, "the folder page, its index, a page below and a query - not /shopping")
	assert.NotContains(t, inFolder, "https://example.com/shopping/b")

	typed := resultMap()
	filter(typed, getFilterChain(Filters{Prefix: "https://example.com/sho"}, false))
	assert.Len(t, typed, 5, "a typed prefix stays a plain string prefix")
}

func TestFindResult(t *testing.T) {
	results := map[string]vo.ScrapeResult{
		"https://user:pw@example.com/a": {TargetURL: "https://user:pw@example.com/a", Code: 200},
		"https://example.com/old": {
			TargetURL: "https://example.com/old",
			Code:      200,
			Redirects: []vo.Redirect{{Code: 301, URL: "https://example.com/new"}},
		},
		"https://example.com/b": {TargetURL: "https://example.com/b", Code: 404},
	}
	find := func(u string) string {
		if r := findResult(results, u); r != nil {
			return r.TargetURL
		}
		return ""
	}
	assert.Equal(t, "https://example.com/b", find("https://example.com/b"), "exact")
	assert.Equal(t, "https://example.com/b", find("https://example.com/b#top"), "without the fragment")
	assert.Equal(t, "https://user:pw@example.com/a", find("https://example.com/a"), "without credentials")
	assert.Equal(t, "https://example.com/old", find("https://example.com/new"), "the final url of a redirect")
	assert.Equal(t, "", find("https://example.com/unknown"))
	assert.Equal(t, "", find("https://other.org/b"))
}

func TestComparableURL(t *testing.T) {
	for input, want := range map[string]string{
		"https://example.com/b#top":             "https://example.com/b",
		"https://user:pw@example.com/a":         "https://example.com/a",
		"https://user:p@ss@example.com/a?x=1#y": "https://example.com/a?x=1",
		"https://example.com/a?mail=a@b.c":      "https://example.com/a?mail=a@b.c",
		"https://example.com":                   "https://example.com",
	} {
		assert.Equal(t, want, comparableURL(input), input)
	}
}

func TestPathFilter(t *testing.T) {
	resultMap := map[string]vo.ScrapeResult{}
	for _, u := range []string{
		"https://example.com/damen/bestseller/cambio",
		"https://example.com/damen/bestseller/allude?q=cambio",
		"https://cambio.example.com/herren",
		"https://example.com/Herren/CAMBIO-jeans",
	} {
		resultMap[u] = vo.ScrapeResult{TargetURL: u}
	}
	filter(resultMap, getFilterChain(Filters{Path: "Cambio"}, false))
	assert.Len(t, resultMap, 2, "case insensitive, anywhere in the path - not in the host or the query")
	assert.Contains(t, resultMap, "https://example.com/damen/bestseller/cambio")
	assert.Contains(t, resultMap, "https://example.com/Herren/CAMBIO-jeans")
}

func TestBucketFilter(t *testing.T) {
	ms := time.Millisecond
	resultMap := map[string]vo.ScrapeResult{
		"a": {TargetURL: "a", Duration: 20 * ms},
		"b": {TargetURL: "b", Duration: 50 * ms},
		"c": {TargetURL: "c", Duration: 99 * ms},
		"d": {TargetURL: "d", Duration: 100 * ms},
	}
	options := getFilterOptions(resultMap)
	if assert.Len(t, options.Buckets, len(vo.GetBucketList()), "all buckets, empty ones too") {
		assert.Equal(t, 1, options.Buckets[0].Pages, "< 50ms")
		assert.Equal(t, 2, options.Buckets[1].Pages, "50ms - 100ms")
		assert.Equal(t, 1, options.Buckets[2].Pages, "100ms - 200ms")
	}
	great := options.Buckets[1]
	filter(resultMap, getFilterChain(Filters{MinDur: great.From, MaxDur: great.To}, false))
	assert.Len(t, resultMap, 2, "the bucket bounds are [from, to)")
	assert.Contains(t, resultMap, "b", "exactly 50ms belongs to the bucket")
	assert.Contains(t, resultMap, "c")
}
