package reports

import (
	"io"

	"github.com/foomo/walker/vo"
)

func reportValidations(status vo.Status, w io.Writer, filter scrapeResultFilter) {
	printh, println, _ := printers(w)
	printh("validations")
	for _, r := range status.Results {
		if filter != nil && filter(r) == false {
			continue
		}
		if len(r.Validations) > 0 {
			println(r.TargetURL)
			for _, v := range r.Validations {
				println("	", v.Group, v.Level, v.Message)
			}
		}
	}
}
