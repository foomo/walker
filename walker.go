package walker

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

type start struct {
	targetURL string
	ignore    []string
}

type Status struct {
	Results map[string]ScrapeResult
	Jobs    map[string]bool
}

type Walker struct {
	concurrency  int
	chanResult   chan ScrapeResult
	chanStart    chan start
	chanStatus   chan Status
	chanErrStart chan error
}

func NewWalker(concurrency int) *Walker {
	w := &Walker{
		concurrency:  concurrency,
		chanResult:   make(chan ScrapeResult),
		chanStart:    make(chan start),
		chanStatus:   make(chan Status),
		chanErrStart: make(chan error),
	}
	go w.main()
	return w
}

func (w *Walker) main() {
	running := 0
	jobs := map[string]bool{}
	results := map[string]ScrapeResult{}
	ignore := []string{}
	var baseURL *url.URL
	for {
		select {
		case st := <-w.chanStart:
			ignore = st.ignore
			startU, errParseStartU := url.Parse(st.targetURL)
			if errParseStartU == nil {
				baseURL = startU
				running = 0
				jobs = map[string]bool{st.targetURL: false}
				results = map[string]ScrapeResult{}
				w.chanErrStart <- nil
			} else {
				w.chanErrStart <- errParseStartU
			}
		case <-w.chanStatus:
			resultsCopy := make(map[string]ScrapeResult, len(results))
			for targetURL, result := range results {
				resultsCopy[targetURL] = result
			}
			jobsCopy := make(map[string]bool, len(jobs))
			for targetURL, active := range jobs {
				jobsCopy[targetURL] = active
			}
			w.chanStatus <- Status{
				Results: resultsCopy,
				Jobs:    jobsCopy,
			}
		case scanResult := <-w.chanResult:
			running--
			// fmt.Println("got a result", scanResult)
			delete(jobs, scanResult.TargetURL)
			results[scanResult.TargetURL] = scanResult

			for linkURL := range scanResult.Links {
				// no paging
				if scanResult.Structure.LinkNext == linkURL ||
					scanResult.Structure.LinkPrev == linkURL {
					// fmt.Println("skipping pager link", linkURL)
					continue
				}

				// ok, time to really look at that url
				linkU, errParseLinkU := normalizeLink(baseURL, linkURL)
				if errParseLinkU == nil {

					// to be ignored ?!
					ignoreLink := false
					for _, ignorePrefix := range ignore {
						if strings.HasPrefix(linkU.Path, ignorePrefix) {
							ignoreLink = true
							break
						}
					}

					if linkU.Host == baseURL.Host &&
						linkU.Scheme == baseURL.Scheme && !ignoreLink {
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
		if len(jobs) > 0 {
			for jobURL, jobActive := range jobs {
				if running >= w.concurrency {
					// fmt.Println("pool exceeded")
					break
				}
				if !jobActive {
					running++
					jobs[jobURL] = true
					go Scrape(jobURL, w.chanResult)
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

func (w *Walker) Walk(targetURL string, ignore ...string) error {
	w.chanStart <- start{
		targetURL: targetURL,
		ignore:    ignore,
	}
	return <-w.chanErrStart
}

func (w *Walker) GetStatus() Status {
	w.chanStatus <- Status{}
	return <-w.chanStatus
}

func line() {
	fmt.Println("------------------------------------------------------------------------")
}
func headline(v ...interface{}) {
	line()
	v = append([]interface{}{"~"}, v...)
	fmt.Println(v...)
	line()
}

func (w *Walker) PrintStatus(status Status) {
	headline("Status: ", " jobs: ", len(status.Jobs), ", results: ", len(status.Results))
	statusCodes := map[int]int{}
	notFoundKeys := []string{}
	notFoundMap := map[string]ScrapeResult{}
	errorKeys := []string{}
	errorMap := map[string]ScrapeResult{}
	for targetURL, result := range status.Results {
		statusCodes[result.Code]++
		switch true {
		case result.Code == 404:
			notFoundKeys = append(notFoundKeys, targetURL)
			notFoundMap[targetURL] = result
		case result.Code > 500:
			errorKeys = append(errorKeys, targetURL)
			errorMap[targetURL] = result
		}
	}
	sort.Strings(errorKeys)
	sort.Strings(notFoundKeys)

	fmt.Println(statusCodes)
	type bucket struct {
		Name string
		From time.Duration
		To   time.Duration
		Show bool
	}

	buckets := []bucket{
		bucket{
			Name: "awesome < 50 ms",
			From: time.Duration(time.Millisecond * 0),
			To:   time.Duration(time.Millisecond * 50),
			Show: false,
		},
		bucket{
			Name: "great < 100 ms",
			From: time.Duration(time.Millisecond * 50),
			To:   time.Duration(time.Millisecond * 100),
		},
		bucket{
			Name: "ok < 200 ms",
			From: time.Duration(time.Millisecond * 100),
			To:   time.Duration(time.Millisecond * 200),
		},
		bucket{
			Name: "not too good < 300 ms",
			From: time.Duration(time.Millisecond * 200),
			To:   time.Duration(time.Millisecond * 300),
			Show: false,
		},
		bucket{
			Name: "meh < 500 ms",
			From: time.Duration(time.Millisecond * 300),
			To:   time.Duration(time.Millisecond * 500),
			Show: false,
		},
		bucket{
			Name: "bad < 1 s",
			From: time.Duration(time.Millisecond * 500),
			To:   time.Duration(time.Millisecond * 1000),
			Show: false,
		},
		bucket{
			Name: "really bad < 3 s",
			From: time.Duration(time.Millisecond * 1000),
			To:   time.Duration(time.Millisecond * 3000),
			Show: false,
		},
		bucket{
			Name: "ouch < 5 s",
			From: time.Duration(time.Millisecond * 3000),
			To:   time.Duration(time.Millisecond * 5000),
			Show: false,
		},
		bucket{
			Name: "catastrophic < 10 s",
			From: time.Duration(time.Millisecond * 5000),
			To:   time.Duration(time.Millisecond * 10000),
			Show: false,
		},
		bucket{
			Name: "end of the world > 10 s",
			From: time.Duration(time.Millisecond * 10000),
			To:   time.Duration(time.Hour),
			Show: false,
		},
	}
	for _, bucket := range buckets {
		bucketI := 0
		fmt.Println(bucket.Name, bucket.From, bucket.To)
		for _, result := range status.Results {
			if result.Duration > bucket.From && result.Duration < bucket.To {
				bucketI++
				if bucket.Show {
					fmt.Println("	", result.Status, result.Duration, result.TargetURL)
				}
			}
		}
		fmt.Println("	-> bucket contains", bucketI)
	}
	headline("currently scanning")
	for targetURL, active := range status.Jobs {
		if active {
			fmt.Println(targetURL)
		}
	}
	headline("bad errors")

	for _, targetURL := range errorKeys {
		result := status.Results[targetURL]
		fmt.Println(result.Code, result.Status, result.TargetURL, result.Error)
		// 	fmt.Println(result.Code, result.Status, result.ContentType, result.Duration, result.Error, result.TargetURL)

	}
	headline("dead links")
	for _, targetURL := range notFoundKeys {
		fmt.Println(targetURL)
		// for _, result := range status.Results {
		// 	for link := range result.Links {
		// 		if link == targetURL {
		// 			fmt.Println("	", result.TargetURL, result.Structure.Title)
		// 		}
		// 	}
		// }
	}

	// for _, result := range status.Results {
	//fmt.Println(result.TargetURL, result.Code, result.Status, result.Error, len(result.Links))
	// }
}
