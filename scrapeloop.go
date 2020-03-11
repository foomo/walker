package walker

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/temoto/robotstxt"
)

type poolClient struct {
	agent  string
	client *http.Client
	busy   bool
}

type clientPool struct {
	agent       string
	concurrency int
	useCookies  bool
	clients     []*poolClient
}

func newClientPool(concurrency int, agent string, useCookies bool) *clientPool {
	clients := make([]*poolClient, concurrency)
	for i := 0; i < concurrency; i++ {
		client := &http.Client{
			Timeout: time.Second * 10,
			Transport: &http.Transport{
				Dial: (&net.Dialer{
					Timeout: 5 * time.Second,
				}).Dial,
				TLSHandshakeTimeout: 5 * time.Second,
			},
		}
		if useCookies {
			cookieJar, _ := cookiejar.New(nil)
			client.Jar = cookieJar
		}
		clients[i] = &poolClient{
			client: client,
			busy:   false,
			agent:  agent,
		}
	}
	return &clientPool{
		agent:       agent,
		concurrency: concurrency,
		clients:     clients,
		useCookies:  useCookies,
	}
}

func (w *Walker) scrapeloop() {
	summaryVec, counterVec, totalCounter, progressGaugeOpen, progressGaugeComplete, counterVecStatus := setupMetrics()
	running := 0
	concurrency := 0
	groupHeader := ""
	ignoreRobots := false
	started := false
	ll := linkLimitations{}
	var jobs map[string]bool
	var results map[string]ScrapeResult
	var baseURL *url.URL
	paths := []string{}
	var cp *clientPool
	var robotsGroup *robotstxt.Group

	restart := func(startURL *url.URL, configPaths []string) {
		started = false
		summaryVec.Reset()
		counterVec.Reset()
		counterVecStatus.Reset()
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
		started = true
	}

	for {
		if started {
			progressGaugeComplete.Set(float64(len(results)))
			progressGaugeOpen.Set(float64(len(jobs)))
			if len(jobs) > 0 {
			JobLoop:
				for jobURL, jobActive := range jobs {
					if running >= concurrency {
						// concurrency limit
						break
					}
					if !jobActive {
						for _, poolClient := range cp.clients {
							if !poolClient.busy {
								running++
								jobs[jobURL] = true
								poolClient.busy = true
								go Scrape(poolClient, jobURL, groupHeader, w.chanResult)
								continue JobLoop
							}
						}
						break JobLoop
					}
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
			restart(baseURL, paths)
		}

		select {
		case <-time.After(time.Millisecond * 1000):
			// make sure we do not get stuck
		case st := <-w.chanStart:
			robotsGroup = nil
			groupHeader = st.conf.GroupHeader
			concurrency = st.conf.Concurrency
			ll.ignorePathPrefixes = st.conf.Ignore
			ll.depth = st.conf.Depth
			ll.paging = st.conf.Paging
			ll.includePathPrefixes = st.conf.Target.Paths
			ignoreRobots = st.conf.IgnoreRobots
			ll.ignoreQueriesWith = st.conf.IgnoreQueriesWith
			ll.ignoreAllQueries = st.conf.IgnoreAllQueries

			if cp == nil || cp.agent != st.conf.Agent || cp.concurrency != st.conf.Concurrency || cp.useCookies != st.conf.UseCookies {
				cp = newClientPool(st.conf.Concurrency, st.conf.Agent, st.conf.UseCookies)
			}

			var errStart error
			startU, errParseStartU := url.Parse(st.conf.Target.BaseURL)
			if errParseStartU != nil {
				errStart = errParseStartU
			}
			if errStart == nil && !ignoreRobots {
				robotsData, errRobotsGroup := getRobotsData(st.conf.Target.BaseURL)
				if errRobotsGroup == nil {
					robotsGroup = robotsData.FindGroup(st.conf.Agent)
					robotForbiddenPath := []string{}
					for _, p := range st.conf.Target.Paths {
						if !robotsGroup.Test(p) {
							robotForbiddenPath = append(robotForbiddenPath, p)
						}
					}
					if len(robotForbiddenPath) > 0 {
						errStart = errors.New("robots.txt does not allow access to the following path (you can either ignore robots or try as a different user agent): " + strings.Join(robotForbiddenPath, ", "))
					}
				} else {
					errStart = errRobotsGroup
				}
			}
			if errStart == nil {
				restart(startU, st.conf.Target.Paths)
				w.chanErrStart <- nil
			} else {
				w.chanErrStart <- errStart
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
			scrapeWindowCount := int64(0)
			now := time.Now()

			first := now.Unix()
			scrapeWindowFirst := now.Unix()
			totalCount := int64(0)

			for _, r := range results {
				// the results are not sorted baby
				totalCount++
				if first > r.Time.Unix() {
					first = r.Time.Unix()
				}
				if now.Sub(r.Time) < scrapeWindow {
					if scrapeWindowFirst > r.Time.Unix() {
						scrapeWindowFirst = r.Time.Unix()
					}
					scrapeWindowCount++
				}
			}
			currentScrapeWindowSeconds := now.Unix() - scrapeWindowFirst
			scrapeTotalSeconds := now.Unix() - first
			w.chanStatus <- Status{
				Results:              resultsCopy,
				ScrapeSpeed:          float64(scrapeWindowCount) / float64(currentScrapeWindowSeconds),
				ScrapeSpeedAverage:   float64(totalCount) / float64(scrapeTotalSeconds),
				ScrapeWindowRequests: scrapeWindowCount,
				ScrapeWindowSeconds:  currentScrapeWindowSeconds,
				ScrapeTotalRequests:  totalCount,
				ScrapeTotalSeconds:   scrapeTotalSeconds,
				Jobs:                 jobsCopy,
			}
		case scanResult := <-w.chanResult:
			running--
			delete(jobs, scanResult.TargetURL)
			scanResult.poolClient.busy = false
			scanResult.Time = time.Now()
			statusCodeAsString := strconv.Itoa(scanResult.Code)
			counterVecStatus.WithLabelValues(statusCodeAsString).Inc()
			results[scanResult.TargetURL] = scanResult

			summaryVec.WithLabelValues(scanResult.Group).Observe(scanResult.Duration.Seconds())
			counterVec.WithLabelValues(scanResult.Group, statusCodeAsString).Inc()
			totalCounter.Inc()

			if ignoreRobots || !strings.Contains(scanResult.Structure.Robots, "nofollow") {
				linkNextNormalized := ""
				linkPrevNormalized := ""
				// should we follow the links
				linkNextNormalizedURL, errNormalizeNext := normalizeLink(baseURL, scanResult.Structure.LinkNext)
				if errNormalizeNext == nil {
					linkNextNormalized = linkNextNormalizedURL.String()
				}
				linkPrevNormalizedURL, errNormalizedPrev := normalizeLink(baseURL, scanResult.Structure.LinkPrev)
				if errNormalizedPrev == nil {
					linkPrevNormalized = linkPrevNormalizedURL.String()
				}

				linksToScrape := filterScrapeLinks(scanResult.Links, baseURL, linkNextNormalized, linkPrevNormalized, ll, robotsGroup)
				for linkToScrape := range linksToScrape {
					// scanResult.Links[linkU.String()] = scanResult.Links[linkURL]
					// linkURL = linkU.String()
					_, existingResultOK := results[linkToScrape]
					_, existingJobOK := jobs[linkToScrape]
					if !existingResultOK && !existingJobOK {
						jobs[linkToScrape] = false
					}
				}

			}

		}
	}
}
