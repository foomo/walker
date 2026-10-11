package walker

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/foomo/walker/htmlschema"
	"github.com/foomo/walker/vo"
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

type contextKeyRedirects struct{}

func getRedirectsFromRequest(r *http.Request) []vo.Redirect {
	via := r.Context().Value(contextKeyRedirects{})
	if via != nil {
		return via.([]vo.Redirect)
	}
	return []vo.Redirect{}
}

func newClientPool(concurrency int, agent string, useCookies bool) *clientPool {
	clients := make([]*poolClient, concurrency)
	for i := 0; i < concurrency; i++ {
		client := &http.Client{
			Timeout: time.Second * 10,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 5 * time.Second,
				}).DialContext,
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   50,
				IdleConnTimeout:       15 * time.Second,
				TLSHandshakeTimeout:   5 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				TLSClientConfig: &tls.Config{
					// disable SSL certificate verification
					InsecureSkipVerify: true,
				},
			},

			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) > 9 {
					return errors.New("stopped after 10 redirects")
				}
				c := req.Context()
				vias := getRedirectsFromRequest(req)
				newR := req.WithContext(
					context.WithValue(
						c,
						contextKeyRedirects{}, append(vias, vo.Redirect{
							Code: req.Response.StatusCode,
							URL:  req.URL.String(),
						})))
				*req = *newR
				return nil
			},
		}
		client.Jar = nil
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
	defer close(w.chanDone)
	running := 0
	concurrency := 0
	groupHeader := ""
	ignoreRobots := false
	scrapeLoopStarted := false
	var chanLoopComplete chan vo.Status
	var scrapeFunc ScrapeFunc
	var validationFunc ValidationFunc
	var linkListFilterFunc LinkListFilterFunc
	var scrapeResultModifierFunc ScrapeResultModifierFunc
	ll := linkLimitations{}
	var jobs map[string]bool
	var results map[string]vo.ScrapeResult
	var baseURL *url.URL
	paths := []string{}
	var cp *clientPool
	var robotsGroup *robotstxt.Group
	var groupValidator *htmlschema.GroupValidator

	// scheduling
	var interval, offset time.Duration
	var cycleStart, cycleStarted, nextCycle, completedAt time.Time
	var waitTimer *time.Timer
	var chanNextCycle <-chan time.Time
	var completeStatus *vo.Status
	deadlineLogged := false
	cycleCompleted := false
	isRunning := func() bool {
		return scrapeLoopStarted && !cycleCompleted
	}
	stopWaiting := func() {
		if waitTimer != nil {
			waitTimer.Stop()
		}
		waitTimer = nil
		chanNextCycle = nil
	}

	restart := func(startURL *url.URL, configPaths []string, slot time.Time) {
		scrapeLoopStarted = false
		stopWaiting()
		cycleStart = slot
		cycleStarted = time.Now()
		cycleCompleted = false
		completedAt = time.Time{}
		nextCycle = time.Time{}
		deadlineLogged = false
		baseURL = startURL
		paths = configPaths
		running = 0
		baseURLString := baseURL.Scheme + "://" + baseURL.Host
		// port := baseURL.Port()
		// if port != "" {
		// 	baseURLString += ":" + port
		// }
		baseURLString += baseURL.RawPath
		q := ""
		if len(baseURL.Query()) > 0 {
			q = "?" + baseURL.RawQuery
		}
		jobs = map[string]bool{}
		for _, p := range paths {
			jobs[baseURLString+p+q] = false
		}

		results = map[string]vo.ScrapeResult{}
		scrapeLoopStarted = true
	}

	getStatus := func() vo.Status {
		resultsCopy := make(map[string]vo.ScrapeResult, len(results))
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
		// a completed cycle is frozen at its end - speeds must not decay while waiting
		now := time.Now()
		if cycleCompleted {
			now = completedAt
		}

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
		status := vo.Status{
			Results:              resultsCopy,
			ScrapeSpeed:          perSecond(scrapeWindowCount, currentScrapeWindowSeconds),
			ScrapeSpeedAverage:   perSecond(totalCount, scrapeTotalSeconds),
			ScrapeWindowRequests: scrapeWindowCount,
			ScrapeWindowSeconds:  currentScrapeWindowSeconds,
			ScrapeTotalRequests:  totalCount,
			ScrapeTotalSeconds:   scrapeTotalSeconds,
			Jobs:                 jobsCopy,
			Running:              isRunning(),
			Started:              cycleStarted,
			Completed:            completedAt,
			Paths:                paths,
		}
		if interval > 0 {
			status.CycleStart = cycleStart
			status.CycleDeadline = cycleStart.Add(interval)
			status.NextCycle = nextCycle
		}
		return status
	}

	for {
		if scrapeLoopStarted {
			if interval > 0 && !deadlineLogged && len(jobs) > 0 && time.Now().After(cycleStart.Add(interval)) {
				deadlineLogged = true
				fmt.Println("ERROR scrape cycle started at", cycleStart, "exceeded interval", interval, "-", len(jobs), "jobs still open, continuing")
			}
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
								go scrape(poolClient, jobURL, baseURL, groupHeader, scrapeFunc, validationFunc, groupValidator, w.chanResult)
								continue JobLoop
							}
						}
						break JobLoop
					}
				}
			}
		}

		// time to restart
		if results != nil && len(jobs) == 0 && running == 0 && baseURL != nil && !cycleCompleted {
			cycleCompleted = true
			completedAt = time.Now()
			cycleDuration := completedAt.Sub(cycleStarted)
			w.metrics.scrapesComplete.Inc()
			overrun := false
			if interval > 0 {
				nextCycle = cycleStart.Add(interval)
				if !completedAt.Before(nextCycle) {
					// the next cycle starts right away
					overrun = true
					nextCycle = completedAt
				}
			}
			status := getStatus()
			completeStatus = &status
			if chanLoopComplete != nil {
				publishLatest(chanLoopComplete, status)
			}
			if interval == 0 {
				fmt.Println("restarting", baseURL, paths)
				restart(baseURL, paths, completedAt)
			} else if !overrun {
				fmt.Println("scrape cycle completed in", cycleDuration, "next cycle at", nextCycle)
				waitTimer = time.NewTimer(nextCycle.Sub(completedAt))
				chanNextCycle = waitTimer.C
			} else {
				w.metrics.overruns.Inc()
				fmt.Println("ERROR scrape cycle started at", cycleStart, "took", cycleDuration, "exceeding interval", interval, "- restarting immediately")
				restart(baseURL, paths, cycleSlot(completedAt, interval, offset))
			}
		}

		select {
		case <-time.After(time.Millisecond * 1000):
			// make sure we do not get stuck
		case <-chanNextCycle:
			fmt.Println("starting scheduled scrape cycle", baseURL, paths)
			restart(baseURL, paths, nextCycle)
		case st := <-w.chanStart:
			robotsGroup = nil
			groupHeader = st.conf.GroupHeader
			concurrency = st.conf.Concurrency
			scrapeFunc = st.scrapeFunc
			validationFunc = st.validationFunc
			linkListFilterFunc = st.linkListFilterFunc
			ll.ignorePathPrefixes = st.conf.Ignore
			ll.depth = st.conf.Depth
			ll.paging = st.conf.Paging
			groupValidator = st.groupValidator
			ll.includePathPrefixes = st.conf.Target.Paths
			ignoreRobots = st.conf.IgnoreRobots
			ll.ignoreQueriesWith = st.conf.IgnoreQueriesWith
			ll.ignoreAllQueries = st.conf.IgnoreAllQueries
			scrapeResultModifierFunc = st.scrapeResultModifierFunc
			interval = st.conf.Interval
			offset = st.conf.Offset

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
				restart(startU, st.conf.Target.Paths, cycleSlot(time.Now(), interval, offset))
				chanLoopComplete = make(chan vo.Status, 1)
				w.chanStarted <- started{
					Err:              errStart,
					ChanLoopComplete: chanLoopComplete,
				}
			} else {
				chanLoopComplete = nil
				w.chanStarted <- started{
					Err: errStart,
				}
			}

		case <-w.chanStatus:
			w.chanStatus <- getStatus()
		case <-w.chanStatuses:
			st := statuses{complete: completeStatus}
			if isRunning() {
				runningStatus := getStatus()
				st.running = &runningStatus
			}
			w.chanStatuses <- st
		case <-w.chanStop:
			w.chanStop <- getStatus()
			return
		case scanResult := <-w.chanResult:
			running--
			delete(jobs, scanResult.result.TargetURL)
			if scrapeResultModifierFunc != nil {
				modifiedScrapeResult, errModify := scrapeResultModifierFunc(scanResult.result)
				if errModify == nil {
					scanResult.result = modifiedScrapeResult
				} else {
					fmt.Println("cound not modify scrape result", errModify)
				}
			}
			scanResult.poolClient.busy = false
			scanResult.result.Time = time.Now()
			results[scanResult.result.TargetURL] = scanResult.result
			w.metrics.requests.WithLabelValues(scanResult.result.Group, strconv.Itoa(scanResult.result.Code)).Inc()

			var linksToScrape vo.LinkList

			if linkListFilterFunc != nil {
				if scanResult.result.Error != "" {
					fmt.Println("there was an error", scanResult.result.Error)
				} else if scanResult.doc != nil {
					linksToScrapeFromFromLilterFunc, errFilterLinkList := linkListFilterFunc(baseURL, scanResult.docURL, scanResult.doc)
					if errFilterLinkList != nil {
						fmt.Println("aua", errFilterLinkList)
					}
					linksToScrape = linksToScrapeFromFromLilterFunc
				}
			} else if ignoreRobots || !strings.Contains(scanResult.result.Structure.Robots, "nofollow") {
				linkNextNormalized := ""
				linkPrevNormalized := ""
				// should we follow the links
				linkNextNormalizedURL, errNormalizeNext := NormalizeLink(baseURL, scanResult.result.Structure.LinkNext)
				if errNormalizeNext == nil {
					linkNextNormalized = linkNextNormalizedURL.String()
				}
				linkPrevNormalizedURL, errNormalizedPrev := NormalizeLink(baseURL, scanResult.result.Structure.LinkPrev)
				if errNormalizedPrev == nil {
					linkPrevNormalized = linkPrevNormalizedURL.String()
				}

				linksToScrape = filterScrapeLinks(scanResult.result.Links, baseURL, linkNextNormalized, linkPrevNormalized, ll, robotsGroup)
			}
			for linkToScrape := range linksToScrape {
				_, existingResultOK := results[linkToScrape]
				_, existingJobOK := jobs[linkToScrape]
				if !existingResultOK && !existingJobOK {
					jobs[linkToScrape] = false
				}
			}
		}
	}
}

// publishLatest never blocks the scrape loop - if nobody has received the
// previous status from the buffered channel yet, it is replaced by the latest
// one. The scrape loop is the only sender, so the second send can not block.
func publishLatest(ch chan vo.Status, status vo.Status) {
	select {
	case ch <- status:
	default:
		select {
		case <-ch:
		default:
		}
		ch <- status
	}
}

func perSecond(count, seconds int64) float64 {
	if seconds <= 0 {
		return 0
	}
	return float64(count) / float64(seconds)
}
