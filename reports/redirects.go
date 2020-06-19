package reports

import (
	"io"
	"sort"
	"strings"

	"github.com/foomo/walker/vo"
)

func reportRedirects(status vo.Status, w io.Writer, filter scrapeResultFilter) {
	printh, println, _ := printers(w)
	printh("redirects")
	redirects := map[int]map[string][]string{}
	for _, r := range status.Results {
		if filter != nil && filter(r) == false {
			continue
		}
		code := 0
		for _, red := range r.Redirects {
			if red.Code > code {
				code = red.Code
			}
		}
		if code == 0 {
			continue
		}
		if redirects[code] == nil {
			redirects[code] = map[string][]string{}
		}
		for _, red := range r.Redirects {
			redirects[code][r.TargetURL] = append(redirects[code][r.TargetURL], red.URL)
		}
	}
	codes := sort.IntSlice{}
	for code := range redirects {
		codes = append(codes, code)
	}
	sort.Sort(codes)
	for _, code := range codes {
		println(code)
		redirectMap := redirects[code]
		targetURLs := []string{}
		for targetURL := range redirectMap {
			targetURLs = append(targetURLs, targetURL)
		}
		sort.Strings(targetURLs)
		for _, targetURL := range targetURLs {
			println("	", targetURL, " => ", strings.Join(redirectMap[targetURL], " => "))
		}
	}
}
