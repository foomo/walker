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
	summaryVec,
		counterVec,
		totalCounter,
		progressGaugeOpen,
		progressGaugeComplete,
		counterVecStatus,
		trackValidationScore,
		trackValidationPenalties := setupMetrics()
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

	restart := func(startURL *url.URL, configPaths []string) {
		scrapeLoopStarted = false
		summaryVec.Reset()
		counterVec.Reset()
		counterVecStatus.Reset()
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
		return vo.Status{
			Results:              resultsCopy,
			ScrapeSpeed:          float64(scrapeWindowCount) / float64(currentScrapeWindowSeconds),
			ScrapeSpeedAverage:   float64(totalCount) / float64(scrapeTotalSeconds),
			ScrapeWindowRequests: scrapeWindowCount,
			ScrapeWindowSeconds:  currentScrapeWindowSeconds,
			ScrapeTotalRequests:  totalCount,
			ScrapeTotalSeconds:   scrapeTotalSeconds,
			Jobs:                 jobsCopy,
		}
	}

	for {
		if scrapeLoopStarted {
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
		if results != nil && len(jobs) == 0 && running == 0 && baseURL != nil {
			fmt.Println("restarting", baseURL, paths)
			w.CompleteStatus = &vo.Status{
				Results: results,
				Jobs:    jobs,
			}
			if chanLoopComplete != nil {
				go reportSchemaValidationMetrics(
					*w.CompleteStatus,
					paths,
					trackValidationPenalties,
					trackValidationScore,
				)
				chanLoopComplete <- *w.CompleteStatus
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
				chanLoopComplete = make(chan vo.Status)
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
			statusCodeAsString := strconv.Itoa(scanResult.result.Code)
			counterVecStatus.WithLabelValues(statusCodeAsString).Inc()
			results[scanResult.result.TargetURL] = scanResult.result

			summaryVec.WithLabelValues(scanResult.result.Group).Observe(scanResult.result.Duration.Seconds())
			counterVec.WithLabelValues(scanResult.result.Group, statusCodeAsString).Inc()
			totalCounter.Inc()

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
