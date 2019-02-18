package walker

import (
	"fmt"
	"io"
	"sort"
	"time"
)

func (w *Walker) PrintStatus(writer io.Writer, status Status) {
	headline(writer, "Status: ", " jobs: ", len(status.Jobs), ", results: ", len(status.Results), "current scapespeed: ", status.ScrapeSpeed, "requests/s")
	statusCodes := map[int]int{}
	notFoundKeys := []string{}
	notFoundMap := map[string]ScrapeResult{}
	errorKeys := []string{}
	errorMap := map[string]ScrapeResult{}
	for targetURL, result := range status.Results {
		statusCodes[result.Code]++
		switch true {
		case result.Code == 404:
			notFoundKeys = append(notFoundKeys, targetURL)
			notFoundMap[targetURL] = result
		case result.Code >= 500:
			errorKeys = append(errorKeys, targetURL)
			errorMap[targetURL] = result
		}
	}
	sort.Strings(errorKeys)
	sort.Strings(notFoundKeys)

	fmt.Fprintln(writer, statusCodes)
	type bucket struct {
		Name string
		From time.Duration
		To   time.Duration
		Show bool
	}

	buckets := []bucket{
		bucket{
			Name: "awesome < 50 ms",
			From: time.Duration(time.Millisecond * 0),
			To:   time.Duration(time.Millisecond * 50),
			Show: false,
		},
		bucket{
			Name: "great < 100 ms",
			From: time.Duration(time.Millisecond * 50),
			To:   time.Duration(time.Millisecond * 100),
		},
		bucket{
			Name: "ok < 200 ms",
			From: time.Duration(time.Millisecond * 100),
			To:   time.Duration(time.Millisecond * 200),
		},
		bucket{
			Name: "not too good < 300 ms",
			From: time.Duration(time.Millisecond * 200),
			To:   time.Duration(time.Millisecond * 300),
			Show: false,
		},
		bucket{
			Name: "meh < 500 ms",
			From: time.Duration(time.Millisecond * 300),
			To:   time.Duration(time.Millisecond * 500),
			Show: false,
		},
		bucket{
			Name: "bad < 1 s",
			From: time.Duration(time.Millisecond * 500),
			To:   time.Duration(time.Millisecond * 1000),
			Show: false,
		},
		bucket{
			Name: "really bad < 3 s",
			From: time.Duration(time.Millisecond * 1000),
			To:   time.Duration(time.Millisecond * 3000),
			Show: false,
		},
		bucket{
			Name: "ouch < 5 s",
			From: time.Duration(time.Millisecond * 3000),
			To:   time.Duration(time.Millisecond * 5000),
			Show: false,
		},
		bucket{
			Name: "catastrophic < 10 s",
			From: time.Duration(time.Millisecond * 5000),
			To:   time.Duration(time.Millisecond * 10000),
			Show: false,
		},
		bucket{
			Name: "end of the world > 10 s",
			From: time.Duration(time.Millisecond * 10000),
			To:   time.Duration(time.Hour),
			Show: false,
		},
	}
	for _, bucket := range buckets {
		bucketI := 0
		fmt.Fprintln(writer, bucket.Name, bucket.From, bucket.To)
		for _, result := range status.Results {
			if result.Duration > bucket.From && result.Duration < bucket.To {
				bucketI++
				if bucket.Show {
					fmt.Fprintln(writer, "	", result.Status, result.Duration, result.TargetURL)
				}
			}
		}
		fmt.Fprintln(writer, "	-> bucket contains", bucketI)
	}
	headline(writer, "currently scanning")
	for targetURL, active := range status.Jobs {
		if active {
			fmt.Fprintln(writer, targetURL)
		}
	}
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
