// Package service is the api of a walker, exposed with gotsrpc on /service
package service

import (
	"sort"
	"strings"
	"time"

	"github.com/foomo/walker/go/internal/config"
	"github.com/foomo/walker/go/internal/crawler"
	"github.com/foomo/walker/go/internal/vo"
)

type service struct {
	walker *crawler.Walker
	// conf of the walker - the schema developer api uses its agent and group header
	conf *config.Config
}

// New is the service of a walker that walks conf - the schema developer and
// semantic scrape calls use its agent and group header
func New(w *crawler.Walker, conf *config.Config) Service {
	return &service{walker: w, conf: conf}
}

func filter(resultMap map[string]vo.ScrapeResult, filterChain filterChain) {
	for targetURL, scrapeResult := range resultMap {
		for _, filterFunc := range filterChain {
			if !filterFunc(scrapeResult) {
				delete(resultMap, targetURL)
				break
			}
		}
	}
}

type filterFunc func(result vo.ScrapeResult) bool
type filterChain []filterFunc

const (
	ScrapeCurrent  = "current"
	ScrapeComplete = "complete"
)

type Filters struct {
	// Scrape selects ScrapeCurrent (default) or ScrapeComplete
	Scrape string
	// Prefix filters by the start of the target url - if it is the Prefix of a
	// folder, it matches the folder and everything below it (see vo.InFolder)
	Prefix string
	// Path matches results, whose path contains it - case insensitive,
	// without scheme, host and query (see vo.URLPath)
	Path   string
	Status []int
	// Groups matches any of the groups - "" is the group of results without one
	Groups []string
	Errors []string
	// MinDur and MaxDur select durations in [MinDur, MaxDur) - like a
	// performance bucket - 0 is not set
	MinDur time.Duration
	MaxDur time.Duration
}

type StatusStats struct {
	Code  int
	Count int
}

type GroupStats struct {
	Group string
	Count int
}

type FilterOptions struct {
	Status []StatusStats
	Groups []GroupStats
	// Buckets are the performance buckets with their number of pages
	Buckets []vo.PerformanceBucket
	MinDur  time.Duration
	MaxDur  time.Duration
}

// getFilterChain - prefixIsFolder switches the prefix filter to vo.InFolder,
// so that "https://example.com/shop" does not match "https://example.com/shopping"
func getFilterChain(filters Filters, prefixIsFolder bool) filterChain {
	chain := filterChain{}
	if filters.Prefix != "" {
		if prefixIsFolder {
			chain = append(chain, func(result vo.ScrapeResult) bool {
				return vo.InFolder(result.TargetURL, filters.Prefix)
			})
		} else {
			chain = append(chain, func(result vo.ScrapeResult) bool {
				return strings.HasPrefix(result.TargetURL, filters.Prefix)
			})
		}
	}
	if filters.Path != "" {
		needle := strings.ToLower(filters.Path)
		chain = append(chain, func(result vo.ScrapeResult) bool {
			return strings.Contains(strings.ToLower(vo.URLPath(result.TargetURL)), needle)
		})
	}
	if len(filters.Status) > 0 {
		chain = append(chain, func(result vo.ScrapeResult) bool {
			for _, status := range filters.Status {
				if result.Code == status {
					return true
				}
			}
			return false
		})
	}
	if len(filters.Groups) > 0 {
		chain = append(chain, func(result vo.ScrapeResult) bool {
			for _, group := range filters.Groups {
				if result.Group == group {
					return true
				}
			}
			return false
		})
	}
	if filters.MaxDur > 0 {
		chain = append(chain, func(result vo.ScrapeResult) bool {
			return result.Duration < filters.MaxDur
		})
	}
	if filters.MinDur > 0 {
		chain = append(chain, func(result vo.ScrapeResult) bool {
			return result.Duration >= filters.MinDur
		})
	}
	return chain
}

func getFilterOptions(resultMap map[string]vo.ScrapeResult) FilterOptions {
	statusMap := map[int]int{}
	groupMap := map[string]int{}
	bucketList := vo.GetBucketList()
	buckets := make([]vo.PerformanceBucket, len(bucketList))
	for i, bucket := range bucketList {
		buckets[i] = vo.PerformanceBucket{Name: bucket.Name, From: bucket.From, To: bucket.To}
	}
	minDur := time.Duration(1000000000000000000)
	maxDur := time.Duration(0)
	for _, result := range resultMap {
		statusMap[result.Code]++
		groupMap[result.Group]++
		buckets[bucketList.Index(result.Duration)].Pages++
		if result.Duration > maxDur {
			maxDur = result.Duration
		}
		if result.Duration < minDur {
			minDur = result.Duration
		}
	}
	statusStats := []StatusStats{}
	for code, count := range statusMap {
		statusStats = append(statusStats, StatusStats{
			Code:  code,
			Count: count,
		})
	}
	groupStats := []GroupStats{}
	for group, count := range groupMap {
		groupStats = append(groupStats, GroupStats{
			Group: group,
			Count: count,
		})
	}
	sort.Slice(groupStats, func(i, j int) bool { return groupStats[i].Group < groupStats[j].Group })
	return FilterOptions{
		Status:  statusStats,
		Groups:  groupStats,
		Buckets: buckets,
		MinDur:  minDur,
		MaxDur:  maxDur,
	}
}

// GetResults returns a page of the filtered results of ScrapeCurrent (default)
// or ScrapeComplete. The folders are the path tree of all results of the
// same scrape - unfiltered and from the same snapshot as the page, so that
// the tree and the results always correspond.
func (s *service) GetResults(
	filters Filters,
	page int,
	pageSize int,
) (filterOptions FilterOptions, results []vo.ScrapeResult, numPages int, folders []vo.Folder) {
	resultMap := s.scrapeResults(filters.Scrape)
	filterOptions = getFilterOptions(resultMap)
	folders = getFolders(resultMap)
	filter(resultMap, getFilterChain(filters, containsFolder(folders, filters.Prefix)))

	i := 0
	results = make([]vo.ScrapeResult, len(resultMap))

	urls := make([]string, len(resultMap))
	for url := range resultMap {
		urls[i] = url
		i++
	}
	sort.Strings(urls)
	for index, url := range urls {
		results[index] = resultMap[url]
	}
	results, numPages = paginate(results, page, pageSize)
	return
}

// scrapeResults returns a copy of the results of ScrapeCurrent (default) or ScrapeComplete
func (s *service) scrapeResults(scrape string) map[string]vo.ScrapeResult {
	if scrape != ScrapeComplete {
		return s.walker.GetStatus().Results
	}
	complete, _ := s.walker.GetStatuses()
	if complete == nil {
		return map[string]vo.ScrapeResult{}
	}
	// the complete status is shared - callers may modify the copy
	results := make(map[string]vo.ScrapeResult, len(complete.Results))
	for targetURL, result := range complete.Results {
		results[targetURL] = result
	}
	return results
}

// containsFolder reports, if prefix is the Prefix of a folder in the tree
func containsFolder(folders []vo.Folder, prefix string) bool {
	for _, f := range folders {
		if f.Prefix == prefix || (vo.InFolder(prefix, f.Prefix) && containsFolder(f.Folders, prefix)) {
			return true
		}
	}
	return false
}

// GetBrokenLinks returns the broken pages of ScrapeCurrent (default) or
// ScrapeComplete - status >= 400 or failed - and the pages linking to them
func (s *service) GetBrokenLinks(scrape string) []vo.BrokenLink {
	return vo.BrokenLinks(s.scrapeResults(scrape))
}

// GetRedirects returns the redirecting urls of ScrapeCurrent (default) or
// ScrapeComplete with their chains and the pages linking to them
func (s *service) GetRedirects(scrape string) []vo.RedirectChain {
	return vo.Redirects(s.scrapeResults(scrape))
}

// GetLinks returns the link counts of every page of ScrapeCurrent (default)
// or ScrapeComplete - GetLinkedFrom has the linking pages of one url
func (s *service) GetLinks(scrape string) []vo.PageLinks {
	return vo.Links(s.scrapeResults(scrape))
}

// GetLinkedFrom returns the pages of ScrapeCurrent (default) or ScrapeComplete
// linking to a url - the totals and groups of all, but at most limit pages
func (s *service) GetLinkedFrom(scrape string, targetURL string, limit int) vo.LinkedFromPage {
	return vo.LinkedFrom(s.scrapeResults(scrape), targetURL, limit)
}

// GetGraph returns the folder graph of ScrapeCurrent (default) or
// ScrapeComplete - navigation links are left out, see vo.GraphOptions
func (s *service) GetGraph(scrape string, options vo.GraphOptions) vo.Graph {
	return vo.BuildGraph(s.scrapeResults(scrape), options)
}

// GetPageGraph returns the neighborhood of a page - the page first, the
// pages linking to it and linked from it, the most linked up to maxNodes
func (s *service) GetPageGraph(scrape string, options vo.GraphOptions, page string, maxNodes int) vo.Graph {
	return vo.BuildPageGraph(s.scrapeResults(scrape), options, page, maxNodes)
}

// GetPageFolderLinks returns the links of a page to the folder nodes of the
// graph built with the same options
func (s *service) GetPageFolderLinks(scrape string, options vo.GraphOptions, page string) []vo.FolderLink {
	return vo.BuildPageFolderLinks(s.scrapeResults(scrape), options, page)
}

// GetFolderPages returns the pages of a folder node of the graph built with
// the same options - at most maxPages, the most linked ones
func (s *service) GetFolderPages(scrape string, options vo.GraphOptions, folder string, maxPages int) vo.FolderPages {
	return vo.BuildFolderPages(s.scrapeResults(scrape), options, folder, maxPages)
}

// GetResult finds the result of ScrapeCurrent (default) or ScrapeComplete for
// a url as a browser sees it: an exact match first, then ignoring the fragment
// and credentials, then a result whose redirects end at the url - nil, if the
// url was not scraped
func (s *service) GetResult(scrape string, targetURL string) *vo.ScrapeResult {
	return findResult(s.scrapeResults(scrape), targetURL)
}

func findResult(results map[string]vo.ScrapeResult, targetURL string) *vo.ScrapeResult {
	if r, ok := results[targetURL]; ok {
		return &r
	}
	key := comparableURL(targetURL)
	var redirected *vo.ScrapeResult
	for _, r := range results {
		if comparableURL(r.TargetURL) == key {
			return &r
		}
		if redirected == nil && len(r.Redirects) > 0 && comparableURL(r.Redirects[len(r.Redirects)-1].URL) == key {
			redirected = &r
		}
	}
	return redirected
}

// comparableURL strips the fragment and credentials - a browser shows neither
// the way walker stores them
func comparableURL(u string) string {
	if i := strings.Index(u, "#"); i >= 0 {
		u = u[:i]
	}
	if schemeEnd := strings.Index(u, "://"); schemeEnd >= 0 {
		hostStart := schemeEnd + 3
		hostEnd := len(u)
		if i := strings.IndexAny(u[hostStart:], "/?"); i >= 0 {
			hostEnd = hostStart + i
		}
		if at := strings.LastIndex(u[hostStart:hostEnd], "@"); at >= 0 {
			u = u[:hostStart] + u[hostStart+at+1:]
		}
	}
	return u
}

// getFolders returns the folder tree of the results - a Folder.Prefix works
// as Filters.Prefix
func getFolders(results map[string]vo.ScrapeResult) []vo.Folder {
	return vo.Folders(results)
}

// paginate returns the given page - pageSize <= 0 returns everything as one page
func paginate(results []vo.ScrapeResult, page, pageSize int) (pageResults []vo.ScrapeResult, numPages int) {
	if pageSize <= 0 {
		return results, 1
	}
	numPages = (len(results) + pageSize - 1) / pageSize
	start := min(max(page, 0)*pageSize, len(results))
	end := min(start+pageSize, len(results))
	return results[start:end], numPages
}

// GetScrapes returns summaries of the last completed scrape and the one in
// progress - either one is nil if there is none
func (s *service) GetScrapes() (complete, current *vo.ScrapeSummary) {
	completeStatus, runningStatus := s.walker.GetStatuses()
	if completeStatus != nil {
		summary := completeStatus.Summary()
		complete = &summary
	}
	if runningStatus != nil {
		summary := runningStatus.Summary()
		current = &summary
	}
	return
}

// RestartScrape starts a new scrape cycle right away - a running one is
// aborted, its results are dropped; the last complete scrape stays
func (s *service) RestartScrape() bool {
	return s.walker.Restart()
}

func (s *service) GetStatus() vo.ServiceStatus {
	walkerStatus := s.walker.GetStatus()
	open := 0
	pending := 0
	for _, active := range walkerStatus.Jobs {
		if active {
			pending++
		} else {
			open++
		}
	}
	return vo.ServiceStatus{
		//		TargetURL: s.targetURL,
		Done:    len(walkerStatus.Results),
		Open:    open,
		Pending: pending,
	}
}
