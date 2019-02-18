package walker

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	yaml "gopkg.in/yaml.v1"
)

type reporter func(status Status, w io.Writer)

func GetReportHandler(basePath string, walker *Walker) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, basePath+"/")
		fmt.Println("handling reports:", path)
		switch true {
		case strings.HasPrefix(path, "broken-links"):
			report(walker, reportBrokenLinks, w)
		case strings.HasPrefix(path, "results"):
			report(walker, reportResults, w)
		case strings.HasPrefix(path, "list"):
			report(walker, reportList, w)
		case strings.HasPrefix(path, "errors"):
			report(walker, reportErrors, w)
		default:
			http.NotFound(w, r)
		}
	}
}

func report(walker *Walker, r reporter, w io.Writer) {
	printh, _, _ := printers(w)
	if walker.CompleteStatus != nil {
		printh("complete status")
		r(*walker.CompleteStatus, w)
	}
	printh("running status")
	r(walker.GetStatus(), w)
}

func printers(w io.Writer) (printh func(header ...interface{}), println func(a ...interface{}), printsep func()) {
	printsep = func() {
		fmt.Fprintln(w, "-----------------------------------------------------------------------------")
	}
	println = func(a ...interface{}) { fmt.Fprintln(w, a...) }
	printh = func(header ...interface{}) {
		printsep()
		println(append([]interface{}{"~"}, header...))
		printsep()
	}
	return
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
