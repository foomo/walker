package walker

import (
	"fmt"
	"io"
	"math"
	"sort"
	"time"

	"github.com/foomo/walker/vo"
)

func groupedBucketListStatus(writer io.Writer, results map[string]vo.ScrapeResult) {
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

	reportSummaryBody(status, writer)

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
