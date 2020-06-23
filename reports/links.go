package reports

import (
	"io"
	"sort"

	"github.com/foomo/walker/vo"
)

func reportLinks(status vo.Status, w io.Writer, filter scrapeResultFilter) {
	printh, println, _ := printers(w)
	printh("links", len(status.Results))
	for _, res := range status.Results {
		if filter != nil && filter(res) == false {
			continue
		}
		println(res.TargetURL)
		links := []string{}
		for _, r := range status.Results {
		LinkLoop:
			for l := range r.NormalizedLinks {
				if l == res.TargetURL {
					links = append(links, r.TargetURL)
					break LinkLoop
				}
			}
		}
		sort.Strings(links)
		for _, l := range links {
			println("	", l)
		}
	}
}
