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
	// Running is true while a scrape cycle is in progress
	Running bool
	// Started is the actual start time of the cycle
	Started time.Time
	// Completed is the end of the cycle - zero while running
	Completed time.Time
	// scheduling - only set when an interval is configured
	CycleStart    time.Time
	CycleDeadline time.Time
	// NextCycle is set, once the cycle has completed
	NextCycle time.Time
}
