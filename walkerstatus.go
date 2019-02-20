package walker

import (
	"fmt"
	"io"
	"sort"
	"time"
)

type bucket struct {
	Name string
	From time.Duration
	To   time.Duration
}

type bucketList []bucket

func getBucketList() bucketList {
	return bucketList{
		bucket{
			Name: "awesome",
			From: time.Duration(time.Millisecond * 0),
			To:   time.Duration(time.Millisecond * 50),
		},
		bucket{
			Name: "great",
			From: time.Duration(time.Millisecond * 50),
			To:   time.Duration(time.Millisecond * 100),
		},
		bucket{
			Name: "ok, google loves you",
			From: time.Duration(time.Millisecond * 100),
			To:   time.Duration(time.Millisecond * 200),
		},
		bucket{
			Name: "not too good, but still ok",
			From: time.Duration(time.Millisecond * 200),
			To:   time.Duration(time.Millisecond * 300),
		},
		bucket{
			Name: "not great",
			From: time.Duration(time.Millisecond * 300),
			To:   time.Duration(time.Millisecond * 500),
		},
		bucket{
			Name: "bad, users start to feel a real difference",
			From: time.Duration(time.Millisecond * 500),
			To:   time.Duration(time.Millisecond * 1000),
		},
		bucket{
			Name: "really bad, you are using users",
			From: time.Duration(time.Millisecond * 1000),
			To:   time.Duration(time.Millisecond * 3000),
		},
		bucket{
			Name: "ouch this seems broken",
			From: time.Duration(time.Millisecond * 3000),
			To:   time.Duration(time.Millisecond * 5000),
		},
		bucket{
			Name: "catastrophic you site seems to be down",
			From: time.Duration(time.Millisecond * 5000),
			To:   time.Duration(time.Millisecond * 10000),
		},
		bucket{
			Name: "end of the world - this must not happen",
			From: time.Duration(time.Millisecond * 10000),
			To:   time.Duration(time.Hour),
		},
	}
}

func bucketListStatus(writer io.Writer, results map[string]ScrapeResult) {
	for _, bucket := range getBucketList() {
		bucketI := 0
		for _, result := range results {
			if result.Duration > bucket.From && result.Duration < bucket.To {
				bucketI++
			}
		}
		fmt.Fprintln(writer, bucketI, "	(", bucket.From, "=>", bucket.To, ")", bucket.Name)
	}

}

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

	bucketListStatus(writer, status.Results)

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
