package walker

import (
	"fmt"
	"io"
	"sort"

	"github.com/foomo/walker/reports"
	"github.com/foomo/walker/vo"
)

func (w *Walker) PrintStatus(writer io.Writer, status vo.Status) {

	headline(writer,
		"Status: ",
		" jobs: ", len(status.Jobs),
		", results: ", len(status.Results),
		", current scapespeed: ", status.ScrapeSpeed, "requests/s",
		",average scrape speed:", status.ScrapeSpeedAverage, "requests/s",
	)
	headline(writer,
		" scrape window: ", status.ScrapeWindowRequests, status.ScrapeWindowSeconds,
		", scapespeed: ", status.ScrapeSpeed, "requests/s",
	)
	headline(writer,
		" scrape average: ", status.ScrapeTotalRequests, status.ScrapeTotalSeconds,
		", scapespeed: ", status.ScrapeSpeedAverage, "requests/s",
	)

	reports.ReportSummaryBody(status, writer, nil)

	headline(writer, "currently scanning")
	for targetURL, active := range status.Jobs {
		if active {
			fmt.Fprintln(writer, targetURL)
		}
	}

	notFoundKeys := []string{}
	notFoundMap := map[string]vo.ScrapeResult{}
	errorKeys := []string{}
	errorMap := map[string]vo.ScrapeResult{}
	for targetURL, result := range status.Results {
		switch true {
		case result.Code == 404:
			notFoundKeys = append(notFoundKeys, targetURL)
			notFoundMap[targetURL] = result
		case result.Code >= 500:
			errorKeys = append(errorKeys, targetURL)
			errorMap[targetURL] = result
		case result.Code == 0:
			errorKeys = append(errorKeys, targetURL)
			errorMap[targetURL] = result
		}
	}
	sort.Strings(errorKeys)
	sort.Strings(notFoundKeys)

	headline(writer, "bad errors")

	for _, targetURL := range errorKeys {
		result := status.Results[targetURL]
		fmt.Fprintln(writer, result.Code, result.Status, result.TargetURL, result.Error)
	}
	headline(writer, "dead links")
	for _, targetURL := range notFoundKeys {
		fmt.Fprintln(writer, targetURL)
	}
}
