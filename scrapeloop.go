package walker

import (
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type poolClient struct {
	client *http.Client
	busy   bool
}

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
	clientPool := []*poolClient{}
	getBucketList()

	const prometheusLabelGroup = "group"
	const prometheusLabelStatus = "status"

	summaryVec := prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "walker_scrape_durations_seconds",
			Help:       "scrape duration whole request time including streaming of body",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{prometheusLabelGroup},
	)

	counterVec := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "walker_scrape_running_total",
			Help: "Number of scrapes in scan.",
		},
		[]string{prometheusLabelGroup, prometheusLabelStatus},
	)

	totalCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "walker_scrape_counter_total",
		Help: "number of scrapes since start of walker",
	})

	progressGaugeOpen := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "walker_progress_gauge_open",
			Help: "progress open to scrape",
		},
	)

	progressGaugeComplete := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "walker_progress_gauge_complete",
			Help: "progress complete scrapes",
		},
	)

	counterVecStatus := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "walker_progress_status_code_total",
		Help: "status codes for running scrape",
	}, []string{prometheusLabelStatus})

	prometheus.MustRegister(
		summaryVec,
		counterVec,
		totalCounter,
		progressGaugeComplete,
		progressGaugeOpen,
		counterVecStatus,
	)

	clientPool = make([]*poolClient, w.concurrency)
	for i := 0; i < w.concurrency; i++ {
		client := &http.Client{
			Timeout: time.Second * 10,
			Transport: &http.Transport{
				Dial: (&net.Dialer{
					Timeout: 5 * time.Second,
				}).Dial,
				TLSHandshakeTimeout: 5 * time.Second,
			},
		}
		if w.useCookies {
			cookieJar, _ := cookiejar.New(nil)
			client.Jar = cookieJar
		}
		clientPool[i] = &poolClient{
			client: client,
			busy:   false,
		}
	}

	start := func(startURL *url.URL, configPaths []string) {
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

	}
	for {

		progressGaugeComplete.Set(float64(len(results)))
		progressGaugeOpen.Set(float64(len(jobs)))
		if len(jobs) > 0 {
		JobLoop:
			for jobURL, jobActive := range jobs {
				if running >= w.concurrency {
					// concurrency limit
					break
				}
				if !jobActive {
					for _, poolClient := range clientPool {
						if !poolClient.busy {
							running++
							jobs[jobURL] = true
							poolClient.busy = true
							// u, _ := url.Parse(jobURL)
							// fmt.Println("got pool client", i, poolClient.client.Jar.Cookies(u))
							go Scrape(poolClient, jobURL, groupHeader, w.chanResult)
							continue JobLoop
						}
					}
					// fmt.Println("all clients are busy")
					break JobLoop
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

		select {
		case <-time.After(time.Millisecond * 1000):
			// make sure we do not get stuck
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
