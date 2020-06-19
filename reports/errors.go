package reports

import (
	"io"
	"sort"

	"github.com/foomo/walker/vo"
)

func reportErrors(status vo.Status, w io.Writer, filter scrapeResultFilter) {
	printh, println, _ := printers(w)
	printh("errors")
	errorBuckets := map[int]map[string]vo.ScrapeResult{}
	codes := sort.IntSlice{}
	for _, res := range status.Results {
		if filter != nil && filter(res) == false {
			continue
		}
		if res.Code >= 400 {
			_, mapOK := errorBuckets[res.Code]
			if !mapOK {
				codes = append(codes, res.Code)
				errorBuckets[res.Code] = map[string]vo.ScrapeResult{}
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
