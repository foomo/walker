package walker

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

func (w *Walker) scrapeloop() {
	running := 0
	depth := 0
	paging := false
	groupHeader := ""
	ignoreAllQueries := false
	ignoreRobots := false
	var jobs map[string]bool
	var results map[string]ScrapeResult
	var ignore []string
	var ignoreQueriesWith []string
	var baseURL *url.URL
	paths := []string{}
	start := func(startURL *url.URL, configPaths []string) {
		baseURL = startURL
		paths = configPaths
		running = 0
		baseURLString := baseURL.Scheme + "://" + baseURL.Host
		port := baseURL.Port()
		if port != "" {
			baseURLString += ":" + port
		}
		baseURLString += baseURL.RawPath
		q := ""
		if len(baseURL.Query()) > 0 {
			q = "?" + baseURL.RawQuery
		}
		for _, p := range paths {
			jobs = map[string]bool{baseURLString + p + q: false}
		}
		results = map[string]ScrapeResult{}
	}
	for {
		select {
		case st := <-w.chanStart:
			groupHeader = st.conf.GroupHeader
			ignore = st.conf.Ignore
			depth = st.conf.Depth
			paging = st.conf.Paging
			ignoreRobots = st.conf.IgnoreRobots
			ignoreQueriesWith = st.conf.IgnoreQueriesWith
			ignoreAllQueries = st.conf.IgnoreAllQueries
			startU, errParseStartU := url.Parse(st.conf.Target.BaseURL)
			if errParseStartU == nil {
				start(startU, st.conf.Target.Paths)
				w.chanErrStart <- nil
			} else {
				w.chanErrStart <- errParseStartU
			}
		case <-w.chanStatus:
			resultsCopy := make(map[string]ScrapeResult, len(results))
			jobsCopy := make(map[string]bool, len(jobs))
			if results != nil {
				for targetURL, result := range results {
					resultsCopy[targetURL] = result
				}
				for targetURL, active := range jobs {
					jobsCopy[targetURL] = active
				}
			}
			scrapeWindowSeconds := 60
			scrapeWindow := time.Second * time.Duration(scrapeWindowSeconds)
			scrapeWindowCount := 0
			now := time.Now()
			//var first time.Time
			//fmt.Println("status", len(results), now, "window", scrapeWindow)
			for _, r := range results {
				// fmt.Println(now, " r.Time", r.Time, "sub", now.Sub(r.Time), "window", scrapeWindow)
				if now.Sub(r.Time) < scrapeWindow {
					scrapeWindowCount++
				}
			}
			w.chanStatus <- Status{
				Results:     resultsCopy,
				ScrapeSpeed: float64(scrapeWindowCount) / float64(scrapeWindowSeconds),
				Jobs:        jobsCopy,
			}
		case scanResult := <-w.chanResult:
			running--
			delete(jobs, scanResult.TargetURL)
			scanResult.Time = time.Now()
			results[scanResult.TargetURL] = scanResult

			if ignoreRobots || !strings.Contains(scanResult.Structure.Robots, "nofollow") {
				// should we follow the links
				for linkURL := range scanResult.Links {

					// is it a pager link
					// this might want to be normalized
					isPagerLink := scanResult.Structure.LinkNext == linkURL || scanResult.Structure.LinkPrev == linkURL
					if !paging && isPagerLink {
						continue
					}

					// ok, time to really look at that url
					linkU, errParseLinkU := normalizeLink(baseURL, linkURL)
					if errParseLinkU == nil {

						// to be ignored ?!
						ignoreLink := false

						if len(linkU.Query()) > 0 {
							// it has a query
							if ignoreAllQueries {
								// no queries in general
								ignoreLink = true
							} else {
								// do we filter a query parameter
							IgnoreLoop:
								for _, ignoreP := range ignoreQueriesWith {
									for pName := range linkU.Query() {
										if pName == ignoreP {
											ignoreLink = true
											break IgnoreLoop
										}
									}
								}
							}
						}
						if !ignoreLink {
							foundPath := false
							for _, p := range paths {
								if strings.HasPrefix(linkU.Path, p) {
									foundPath = true
									break
								}
							}
							if !foundPath {
								// not in the scrape path
								ignoreLink = true
							}
						}
						if !ignoreLink && depth > 0 {
							// too deep?
							linkDepth := len(strings.Split(linkU.Path, "/")) - 1
							ignoreLink = linkDepth > depth
							if ignoreLink {
								fmt.Println("ignoring", linkU.Path, depth, linkDepth)
							}
						}
						// ignore prefix
						if !ignoreLink {
							for _, ignorePrefix := range ignore {
								if strings.HasPrefix(linkU.Path, ignorePrefix) {
									ignoreLink = true
									break
								}
							}
						}

						if !ignoreLink && linkU.Host == baseURL.Host &&
							linkU.Scheme == baseURL.Scheme {
							scanResult.Links[linkU.String()] = scanResult.Links[linkURL]
							linkURL = linkU.String()
							_, existingResultOK := results[linkURL]
							_, existingJobOK := jobs[linkURL]
							if !existingResultOK && !existingJobOK {
								jobs[linkURL] = false
							}
						}
					}
				}
			}
		}
		if len(jobs) > 0 {
			for jobURL, jobActive := range jobs {
				if running >= w.concurrency {
					// fmt.Println("pool exceeded")
					break
				}
				if !jobActive {
					running++
					jobs[jobURL] = true
					go Scrape(jobURL, groupHeader, w.chanResult)
				}
			}
		}
		// time to restart
		if results != nil && len(jobs) == 0 && running == 0 && baseURL != nil {
			fmt.Println("restarting", baseURL, paths)
			w.CompleteStatus = &Status{
				Results: results,
				Jobs:    jobs,
			}
			start(baseURL, paths)
		}
	}
}

func normalizeLink(baseURL *url.URL, linkURL string) (normalizedLink *url.URL, err error) {
	// let us ditch anchors
	anchorParts := strings.Split(linkURL, "#")
	linkURL = anchorParts[0]
	link, errParseLink := url.Parse(linkURL)
	if errParseLink != nil {
		err = errParseLink
		return
	}
	// host
	if link.Host == "" {
		link.Host = baseURL.Host
	}
	// scheme
	if link.Scheme == "" || link.Scheme == "//" {
		link.Scheme = baseURL.Scheme
	}
	if baseURL.User != nil {
		link.User = baseURL.User
	}
	// it is beautiful now
	normalizedLink = link
	return
}
