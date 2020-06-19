package reports

import (
	"io"

	"github.com/foomo/walker/vo"
	"gopkg.in/yaml.v3"
)

func reportResults(status vo.Status, w io.Writer, filter scrapeResultFilter) {
	printh, println, _ := printers(w)
	printh("results", len(status.Results))
	for _, res := range status.Results {
		if filter != nil && filter(res) == false {
			continue
		}
		yamlBytes, errYaml := yaml.Marshal(res)
		if errYaml != nil {
			println("could not print", res, errYaml)
		} else {
			println(string(yamlBytes))
		}
	}
}
