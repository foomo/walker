package reports

import (
	"io"
	"net/http"
	"sort"

	"github.com/foomo/walker/vo"
)

func reportBrokenLinks(status vo.Status, w io.Writer, filter scrapeResultFilter) {
	printh, println, _ := printers(w)
	printh("broken links")
	broken := map[string][]string{}
	// collect 404s
	for _, res := range status.Results {
		if filter != nil && filter(res) == false {
			continue
		}
		if res.Code == http.StatusNotFound {
			//println(res.TargetURL)
			broken[res.TargetURL] = []string{}
		}
	}
	// see where they link from
	for _, res := range status.Results {
		if filter != nil && filter(res) == false {
			continue
		}
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
