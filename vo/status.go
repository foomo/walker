package vo

import "time"

type Status struct {
	Results              map[string]ScrapeResult
	Jobs                 map[string]bool
	ScrapeSpeed          float64
	ScrapeSpeedAverage   float64
	ScrapeWindowRequests int64
	ScrapeWindowSeconds  int64
	ScrapeTotalRequests  int64
	ScrapeTotalSeconds   int64
	// scheduling - only set when an interval is configured
	CycleStart    time.Time
	CycleDeadline time.Time
	Waiting       bool
	NextCycle     time.Time
}
