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
	// Paths are the target paths of the cycle
	Paths []string
}

// ScrapeSummary is a Status without the results and jobs - timestamps,
// that are not set, are nil
type ScrapeSummary struct {
	Running            bool
	Started            *time.Time
	Completed          *time.Time
	CycleStart         *time.Time
	CycleDeadline      *time.Time
	NextCycle          *time.Time
	Pages              int
	JobsOpen           int
	StatusCodes        map[int]int
	ScrapeSpeed        float64
	ScrapeSpeedAverage float64
	Paths              []string
	Performance        []GroupPerformance
}

func (s Status) Summary() ScrapeSummary {
	statusCodes := map[int]int{}
	for _, r := range s.Results {
		statusCodes[r.Code]++
	}
	return ScrapeSummary{
		Running:            s.Running,
		Started:            optionalTime(s.Started),
		Completed:          optionalTime(s.Completed),
		CycleStart:         optionalTime(s.CycleStart),
		CycleDeadline:      optionalTime(s.CycleDeadline),
		NextCycle:          optionalTime(s.NextCycle),
		Pages:              len(s.Results),
		JobsOpen:           len(s.Jobs),
		StatusCodes:        statusCodes,
		ScrapeSpeed:        s.ScrapeSpeed,
		ScrapeSpeedAverage: s.ScrapeSpeedAverage,
		Paths:              s.Paths,
		Performance:        Performance(s.Results),
	}
}

func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
