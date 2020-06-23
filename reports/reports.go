package reports

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/foomo/walker/vo"
)

type scrapeResultFilter func(res vo.ScrapeResult) bool
type reporter func(status vo.Status, w io.Writer, filter scrapeResultFilter)

func GetReportHandlerMenuHTML(basePath string) string {
	return `
	<p>Walker standard report handler menu</p>
	<ul>
		<li><a href="` + basePath + `/summary">summary of status codes and performance overview</a></li>
		<li><a href="` + basePath + `/results">all plain results (this can be a very long doc)</a></li>
		<li><a href="` + basePath + `/list">list of all jobs / results</a></li>
		<li><a href="` + basePath + `/highscore">highscore - all results sorted by request duration</a></li>
		<li><a href="` + basePath + `/broken-links">broken links</a></li>
		<li><a href="` + basePath + `/seo">seo</a></li>
		<li><a href="` + basePath + `/redirects">redirects</a></li>
		<li><a href="` + basePath + `/schema">schema</a></li>
		<li><a href="` + basePath + `/validations">validations</a></li>
		<li><a href="` + basePath + `/errors">errors - calls that returned error status codes</a></li>
		<li><a href="` + basePath + `/links">links where are pages being linked from</a></li>
	</ul>
	<p>query parameters</p>
	<table>
		<tr>
			<td>url paramter</td>
			<td>function</td>
			<td>examples</td>
		</tr>
		<tr>
			<td>status</td>
			<td>show only given statuses in given order</td>
			<td>?status=complete,running</td>
		</tr>
		<tr>
			<td>url</td>
			<td>filter only for that one url</td>
			<td>?url=http...</td>
		</tr>
		<tr>
			<td>prefix</td>
			<td>filter all urls with given prefix</td>
			<td>?prefix=http...</td>
		</tr>
	</table>
	`
}

func GetReportHandler(basePath string) func(
	w http.ResponseWriter, r *http.Request,
	completeStatus, runningStatus *vo.Status,
) {
	return func(
		w http.ResponseWriter, r *http.Request,
		completeStatus, runningStatus *vo.Status,
	) {
		path := strings.TrimPrefix(r.URL.Path, basePath+"/")
		fmt.Println("handling reports:", path)
		var rep reporter
		var f scrapeResultFilter
		switch true {
		case strings.HasPrefix(path, "seo"):
			rep = reportSEO
		case strings.HasPrefix(path, "broken-links"):
			rep = reportBrokenLinks
		case strings.HasPrefix(path, "results"):
			rep = reportResults
		case strings.HasPrefix(path, "list"):
			rep = reportList
		case strings.HasPrefix(path, "highscore"):
			rep = reportHighscore
		case strings.HasPrefix(path, "summary"):
			rep = reportSummary
		case strings.HasPrefix(path, "errors"):
			rep = reportErrors
		case strings.HasPrefix(path, "validations"):
			rep = reportValidations
		case strings.HasPrefix(path, "schema"):
			rep = reportSchema
		case strings.HasPrefix(path, "redirects"):
			rep = reportRedirects
		case strings.HasPrefix(path, "links"):
			rep = reportLinks
		default:
			http.NotFound(w, r)
			return
		}
		url := r.URL.Query().Get("url")
		if url != "" {
			f = func(res vo.ScrapeResult) bool {
				return res.TargetURL == url
			}
		}
		prefix := r.URL.Query().Get("prefix")
		if prefix != "" {
			f = func(res vo.ScrapeResult) bool {
				return strings.HasPrefix(res.TargetURL, prefix)
			}
		}

		rawStatuses := strings.Split(r.URL.Query().Get("status"), ",")
		statuses := []string{}
		for _, rawStatus := range rawStatuses {
			rawStatus = strings.Trim(rawStatus, " 	\n")
			switch rawStatus {
			case statusComplete, statusRunning:
				statuses = append(statuses, rawStatus)
			}
		}
		if len(statuses) == 0 {
			statuses = []string{statusRunning, statusComplete}
		}
		report(rep, w, f, statuses, completeStatus, runningStatus)
	}
}

const (
	statusRunning  string = "running"
	statusComplete string = "complete"
)

func report(
	r reporter, w io.Writer, filter scrapeResultFilter,
	statuses []string, completeStatus, runningStatus *vo.Status,
) {
	_, println, _ := printers(w)
	for _, statusName := range statuses {
		var status *vo.Status
		switch statusName {
		case statusRunning:
			status = runningStatus
		case statusComplete:
			status = completeStatus
		}
		if status != nil {
			println("STATUS", statusName)
			println("=============================================================================")
			r(*status, w, filter)
			println()
			println()
		} else {
			println("STATUS", statusName, "is nil")
		}
	}
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
	values := make([]string, len(d))
	i := 0
	for value := range d {
		values[i] = value
		i++
	}
	sort.Strings(values)
	for _, value := range values {
		urls := d[value]
		sort.Strings(urls)
		if len(urls) > 1 {
			println(value)
			for _, url := range urls {
				println("	", url)
			}
		}
	}
}

type uniqueList []string

func (ul *uniqueList) add(v string) {
	for _, ev := range *ul {
		if ev == v {
			return
		}
	}
	*ul = append(*ul, v)
}

func getFinalURLForScrapeResult(r vo.ScrapeResult) string {
	finalURL := r.TargetURL
	if len(r.Redirects) > 0 {
		finalURL = r.Redirects[len(r.Redirects)-1].URL
	}
	return finalURL
}

func reportList(status vo.Status, w io.Writer, filter scrapeResultFilter) {
	printh, println, _ := printers(w)
	printh("results", len(status.Results))
	results := make([]string, len(status.Results))
	i := 0
	for _, res := range status.Results {
		if filter != nil && filter(res) == false {
			continue
		}
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

func reportSummary(status vo.Status, w io.Writer, filter scrapeResultFilter) {
	printh, _, _ := printers(w)
	printh("summary")
	ReportSummaryBody(status, w, filter)
}

func ReportSummaryBody(status vo.Status, w io.Writer, filter scrapeResultFilter) {
	printh, println, _ := printers(w)
	printh("status codes")
	statusMap := map[int]int{}
	for _, r := range status.Results {
		if filter != nil && filter(r) == false {
			continue
		}
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

func groupedBucketListStatus(
	writer io.Writer,
	results map[string]vo.ScrapeResult,
) {
	max := int64(0)
	min := ^int64(0)
	groups := map[string]int64{}
	for _, r := range results {
		groups[r.Group]++
	}
	i := 0
	groupNames := make([]string, len(groups))

	for group := range groups {
		groupNames[i] = group
		i++
	}
	sort.Strings(groupNames)
	for _, groupName := range groupNames {
		fmt.Fprintln(writer, "group: "+groupName)
		for _, bucket := range vo.GetBucketList() {
			bucketI := 0
			for _, result := range results {
				if result.Group == groupName {
					ts := int64(result.Time.Unix())
					if ts > min {
						min = ts
					}
					if ts < max {
						max = ts
					}
					if result.Duration > bucket.From && result.Duration < bucket.To {
						bucketI++
					}
				}
			}
			fmt.Fprintln(
				writer,
				bucketI,
				"	",
				math.Round(float64(bucketI)/float64(groups[groupName])*100),
				"%	(", bucket.From, "=>", bucket.To, ")",
				bucket.Name,
			)
		}
	}
	fmt.Fprintln(writer, "=>", min, time.Unix(min, 0), max, time.Unix(max, 0))
}
