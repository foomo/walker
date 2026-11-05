package reports

import (
	"io"

	"github.com/foomo/walker/go/internal/vo"
)

func reportShape(status vo.Status, w io.Writer, filter scrapeResultFilter) {
	printh, println, _ := printers(w)
	if filter == nil {
		printh("results", len(status.Results))
	} else {
		printh("filtered results")
	}
	for _, res := range status.Results {
		if filter != nil && filter(res) == false {
			continue
		}
		if res.ValidationReport == nil {
			println("no validation report for:", res.TargetURL, res.ValidationError)
			continue
		}
		println("validation report for:", res.TargetURL)
		res.ValidationReport.Print(w)
	}
}
