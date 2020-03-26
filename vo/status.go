package vo

type Status struct {
	Results              map[string]ScrapeResult
	Jobs                 map[string]bool
	ScrapeSpeed          float64
	ScrapeSpeedAverage   float64
	ScrapeWindowRequests int64
	ScrapeWindowSeconds  int64
	ScrapeTotalRequests  int64
	ScrapeTotalSeconds   int64
}
