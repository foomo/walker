package vo

import (
	"math"
	"sort"
	"time"
)

type PerformanceBucket struct {
	Name  string
	From  time.Duration
	To    time.Duration
	Pages int
}

// GroupPerformance is the request duration distribution of a group
type GroupPerformance struct {
	Group string
	Pages int
	// Buckets contains every bucket of GetBucketList in order, empty ones too
	Buckets []PerformanceBucket
	P50     time.Duration
	P90     time.Duration
	Max     time.Duration
}

// Performance distributes the results of every group over the performance
// buckets - sorted by group
func Performance(results map[string]ScrapeResult) []GroupPerformance {
	durations := map[string][]time.Duration{}
	for _, r := range results {
		durations[r.Group] = append(durations[r.Group], r.Duration)
	}
	bucketList := GetBucketList()
	performance := make([]GroupPerformance, 0, len(durations))
	for group, groupDurations := range durations {
		sort.Slice(groupDurations, func(i, j int) bool { return groupDurations[i] < groupDurations[j] })
		gp := GroupPerformance{
			Group:   group,
			Pages:   len(groupDurations),
			Buckets: make([]PerformanceBucket, len(bucketList)),
			P50:     percentile(groupDurations, 0.5),
			P90:     percentile(groupDurations, 0.9),
			Max:     groupDurations[len(groupDurations)-1],
		}
		for i, bucket := range bucketList {
			gp.Buckets[i] = PerformanceBucket{Name: bucket.Name, From: bucket.From, To: bucket.To}
		}
		for _, d := range groupDurations {
			gp.Buckets[bucketList.Index(d)].Pages++
		}
		performance = append(performance, gp)
	}
	sort.Slice(performance, func(i, j int) bool { return performance[i].Group < performance[j].Group })
	return performance
}

// percentile of sorted durations, nearest rank
func percentile(sorted []time.Duration, p float64) time.Duration {
	rank := int(math.Ceil(p*float64(len(sorted)))) - 1
	return sorted[min(max(rank, 0), len(sorted)-1)]
}
