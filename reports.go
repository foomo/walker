package walker

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v1"
)

type reporter func(status Status, w io.Writer)

func GetReportHandler(basePath string, walker *Walker) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, basePath+"/")
		fmt.Println("handling reports:", path)
		switch true {
		case strings.HasPrefix(path, "seo"):
			report(walker, reportSEO, w)
		case strings.HasPrefix(path, "broken-links"):
			report(walker, reportBrokenLinks, w)
		case strings.HasPrefix(path, "results"):
			report(walker, reportResults, w)
		case strings.HasPrefix(path, "list"):
			report(walker, reportList, w)
		case strings.HasPrefix(path, "highscore"):
			report(walker, reportHighscore, w)
		case strings.HasPrefix(path, "summary"):
			report(walker, reportSummary, w)
		case strings.HasPrefix(path, "errors"):
			report(walker, reportErrors, w)
		default:
			http.NotFound(w, r)
		}
	}
}

func report(walker *Walker, r reporter, w io.Writer) {
	_, println, _ := printers(w)
	if walker.CompleteStatus != nil {
		println("COMPLETE STATUS")
		println("=============================================================================")
		r(*walker.CompleteStatus, w)
		println()
		println()
	}
	println("=============================================================================")
	println("RUNNING STATUS")
	println("=============================================================================")
	r(walker.GetStatus(), w)
}

func printers(w io.Writer) (printh func(header ...interface{}), println func(a ...interface{}), printsep func()) {
	printsep = func() {
		fmt.Fprintln(w, "-----------------------------------------------------------------------------")
	}
	println = func(a ...interface{}) { fmt.Fprintln(w, a...) }
	printh = func(header ...interface{}) {
		println()
		println(header...)
		printsep()
	}
	return
}

type duplications map[string][]string

func (d duplications) add(value, url string) {
	existingURLs, ok := d[value]
	if ok {
		for _, existingURL := range existingURLs {
			if existingURL == url {
				return
			}
		}
	}
	d[value] = append(d[value], url)
}

func (d duplications) printlnDuplications(w io.Writer) {
	_, println, _ := printers(w)
	for value, urls := range d {
		if len(urls) > 1 {
			println(value)
			for _, url := range urls {
				println("	", url)
			}
		}
	}
}

type uniqueList []string

func (ul uniqueList) add(v string) {
	for _, ev := range ul {
		if ev == v {
			return
		}
	}
	ul = append(ul, v)
}

func reportSEO(status Status, w io.Writer) {
	printh, println, _ := printers(w)
	h1s := duplications{}
	titles := duplications{}
	descriptions := duplications{}
	missingTitles := uniqueList{}
	missingH1 := uniqueList{}
	missingDescriptions := uniqueList{}
	printh("SEO duplications")
	for _, r := range status.Results {
		if strings.Contains(r.ContentType, "html") {
			foundH1 := false
			for _, heading := range r.Structure.Headings {
				if r.Structure.Title == "" {
					missingTitles.add(r.TargetURL)
				} else {
					titles.add(r.Structure.Title, r.TargetURL)
				}
				if r.Structure.Description == "" {
					descriptions.add(r.Structure.Description, r.TargetURL)
				} else {
					missingDescriptions.add(r.TargetURL)
				}
				if heading.Level == 1 && heading.Text != "" {
					h1s.add(heading.Text, r.TargetURL)
					foundH1 = true
				}
			}
			if !foundH1 {
				missingH1.add(r.TargetURL)
			}
		}
	}
	printDuplicates := func(title string, d duplications) {
		if len(d) > 0 {
			printh(title)
			d.printlnDuplications(w)
		}
	}
	printDuplicates("duplicate h1", h1s)
	printDuplicates("duplicate titles", titles)
	printDuplicates("duplicate descriptions", descriptions)

	printList := func(name string, list []string) {
		if len(list) > 0 {
			printh(name)
			sort.Strings(list)
			for _, l := range list {
				println("	", l)
			}
		}
	}
	printList("missing titles", missingTitles)
	printList("missing descriptions", missingDescriptions)
	printList("missing h1", missingH1)
}

func reportBrokenLinks(status Status, w io.Writer) {
	printh, println, _ := printers(w)
	printh("broken links")
	broken := map[string][]string{}
	// collect 404s
	for _, res := range status.Results {
		if res.Code == http.StatusNotFound {
			//println(res.TargetURL)
			broken[res.TargetURL] = []string{}
		}
	}
	// see where they link from
	for _, res := range status.Results {
		for l := range res.Links {
			for link := range broken {
				if link == l {
					broken[l] = append(broken[l], res.TargetURL)
				}
			}
		}
	}
	// spit it out
	brokenKeys := make([]string, len(broken))
	i := 0
	for k, links := range broken {
		sort.Strings(links)
		brokenKeys[i] = k
		i++
	}
	sort.Strings(brokenKeys)
	for _, brokenKey := range brokenKeys {
		println(brokenKey, " (", len(broken[brokenKey]), "):")
		for i, from := range broken[brokenKey] {
			if i > 19 {
				println("	...")
				break
			}
			println("	", from)
		}
	}
}

func reportResults(status Status, w io.Writer) {
	printh, println, _ := printers(w)
	printh("results", len(status.Results))
	for _, res := range status.Results {
		yamlBytes, errYaml := yaml.Marshal(res)
		if errYaml != nil {
			println("could not print", res, errYaml)
		} else {
			println(string(yamlBytes))
		}
	}
}

func reportList(status Status, w io.Writer) {
	printh, println, _ := printers(w)
	printh("results", len(status.Results))
	results := make([]string, len(status.Results))
	i := 0
	for _, res := range status.Results {
		results[i] = strconv.Itoa(res.Code) + " " + res.TargetURL
		i++
	}
	sort.Strings(results)
	for i, r := range results {
		println(i, r)
	}
	printh("open jobs")
	jobs := []string{}
	for url, done := range status.Jobs {
		if !done {
			jobs = append(jobs, url)
		}
	}
	sort.Strings(jobs)
	for i, url := range jobs {
		println(i, url)
	}
}

func reportErrors(status Status, w io.Writer) {
	printh, println, _ := printers(w)
	printh("errors")
	errorBuckets := map[int]map[string]ScrapeResult{}
	codes := sort.IntSlice{}
	for _, res := range status.Results {
		if res.Code >= 400 {
			_, mapOK := errorBuckets[res.Code]
			if !mapOK {
				codes = append(codes, res.Code)
				errorBuckets[res.Code] = map[string]ScrapeResult{}
			}
			errorBuckets[res.Code][res.TargetURL] = res
		}
	}
	sort.Sort(codes)
	for _, code := range codes {
		println(code, ":")
		urls := make([]string, len(errorBuckets[code]))
		i := 0
		for targetURL := range errorBuckets[code] {
			urls[i] = targetURL
			i++
		}
		sort.Strings(urls)
		for _, url := range urls {
			println("	", url)
		}
	}
}

type score struct {
	TargetURL string
	Code      int
	Duration  time.Duration
}

type scores []score

func (s scores) Len() int           { return len(s) }
func (s scores) Less(i, j int) bool { return s[i].Duration < s[j].Duration }
func (s scores) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func reportHighscore(status Status, w io.Writer) {
	printh, println, _ := printers(w)
	printh("high score")
	scores := make(scores, len(status.Results))
	i := 0
	for _, r := range status.Results {
		scores[i] = score{
			Duration:  r.Duration,
			Code:      r.Code,
			TargetURL: r.TargetURL,
		}
		i++
	}
	sort.Sort(scores)
	for i, s := range scores {
		println(i, s.Code, s.TargetURL, s.Duration)
	}
}

func reportSummary(status Status, w io.Writer) {
	printh, _, _ := printers(w)
	printh("summary")
	reportSummaryBody(status, w)
}

func reportSummaryBody(status Status, w io.Writer) {
	printh, println, _ := printers(w)
	printh("status codes")
	statusMap := map[int]int{}
	for _, r := range status.Results {
		statusMap[r.Code]++
	}
	codes := sort.IntSlice{}
	for code := range statusMap {
		codes = append(codes, code)
	}
	sort.Sort(codes)
	for _, code := range codes {
		println(code, statusMap[code])
	}
	printh("performance buckets")
	groupedBucketListStatus(w, status.Results)
}
