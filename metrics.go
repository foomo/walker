package walker

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/foomo/walker/vo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	labelScrape         = "scrape"
	labelGroup          = "group"
	labelStatus         = "status"
	labelPath           = "path"
	labelValidationType = "type"

	scrapeCurrent  = "current"
	scrapeComplete = "complete"
)

// metrics are owned by a walker - the counters are updated by the scrape
// loop, everything else is derived from the walker statuses on collection
type metrics struct {
	registry        *prometheus.Registry
	requests        *prometheus.CounterVec
	scrapesComplete prometheus.Counter
	overruns        prometheus.Counter
}

func newMetrics(getStatuses func() (complete, running *vo.Status)) *metrics {
	m := &metrics{
		registry: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "walker_requests_total",
			Help: "requests since start of walker",
		}, []string{labelGroup, labelStatus}),
		scrapesComplete: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "walker_scrapes_completed_total",
			Help: "completed scrape cycles since start of walker",
		}),
		overruns: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "walker_scrape_overruns_total",
			Help: "scrape cycles, that took longer than the configured interval",
		}),
	}
	m.registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.requests,
		m.scrapesComplete,
		m.overruns,
		newScrapeCollector(getStatuses),
	)
	return m
}

// MetricsHandler serves the prometheus metrics of this walker
func (w *Walker) MetricsHandler() http.Handler {
	return promhttp.HandlerFor(w.metrics.registry, promhttp.HandlerOpts{})
}

var (
	descScrapeRunning = prometheus.NewDesc(
		"walker_scrape_running",
		"1 while a scrape cycle is in progress, 0 while waiting for the next one",
		nil, nil,
	)
	descScrapeNextStart = prometheus.NewDesc(
		"walker_scrape_next_start_timestamp_seconds",
		"unix timestamp of the next scheduled scrape cycle - only while waiting for it",
		nil, nil,
	)
	descScrapePages = prometheus.NewDesc(
		"walker_scrape_pages",
		"scraped pages by group and http status code",
		[]string{labelScrape, labelGroup, labelStatus}, nil,
	)
	descScrapeJobsOpen = prometheus.NewDesc(
		"walker_scrape_jobs_open",
		"pages, that are queued or being scraped",
		[]string{labelScrape}, nil,
	)
	descScrapeRequestDuration = prometheus.NewDesc(
		"walker_scrape_request_duration_seconds",
		"request duration including streaming of the body",
		[]string{labelScrape, labelGroup}, nil,
	)
	descScrapeStarted = prometheus.NewDesc(
		"walker_scrape_started_timestamp_seconds",
		"unix timestamp of the start of the scrape cycle",
		[]string{labelScrape}, nil,
	)
	descScrapeCompleted = prometheus.NewDesc(
		"walker_scrape_completed_timestamp_seconds",
		"unix timestamp of the end of the scrape cycle",
		[]string{labelScrape}, nil,
	)
	descScrapeDuration = prometheus.NewDesc(
		"walker_scrape_duration_seconds",
		"duration of the scrape cycle - elapsed time while it is running",
		[]string{labelScrape}, nil,
	)
	descValidationScore = prometheus.NewDesc(
		"walker_scrape_validation_score",
		"average html schema score of the validated pages",
		[]string{labelScrape, labelGroup, labelPath}, nil,
	)
	descValidationPenalty = prometheus.NewDesc(
		"walker_scrape_validation_penalty",
		"average html schema penalty of the validated pages by validation type",
		[]string{labelScrape, labelGroup, labelPath, labelValidationType}, nil,
	)
)

// durationBuckets are the upper bounds of the report performance buckets
func durationBuckets() []float64 {
	bucketList := vo.GetBucketList()
	// the last bucket is open ended => +Inf
	buckets := make([]float64, 0, len(bucketList)-1)
	for _, bucket := range bucketList[:len(bucketList)-1] {
		buckets = append(buckets, bucket.To.Seconds())
	}
	return buckets
}

type scrapeCollector struct {
	getStatuses func() (complete, running *vo.Status)
	buckets     []float64

	// a completed status never changes - its metrics are computed once
	mu              sync.Mutex
	completeSeen    time.Time
	completeMetrics []prometheus.Metric
}

func newScrapeCollector(getStatuses func() (complete, running *vo.Status)) *scrapeCollector {
	return &scrapeCollector{
		getStatuses: getStatuses,
		buckets:     durationBuckets(),
	}
}

func (c *scrapeCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{
		descScrapeRunning,
		descScrapeNextStart,
		descScrapePages,
		descScrapeJobsOpen,
		descScrapeRequestDuration,
		descScrapeStarted,
		descScrapeCompleted,
		descScrapeDuration,
		descValidationScore,
		descValidationPenalty,
	} {
		ch <- d
	}
}

func (c *scrapeCollector) Collect(ch chan<- prometheus.Metric) {
	complete, running := c.getStatuses()
	isRunning := 0.0
	if running != nil {
		isRunning = 1
		for _, m := range c.scrapeMetrics(scrapeCurrent, *running) {
			ch <- m
		}
	}
	ch <- prometheus.MustNewConstMetric(descScrapeRunning, prometheus.GaugeValue, isRunning)
	if complete == nil {
		return
	}
	if running == nil && !complete.NextCycle.IsZero() {
		ch <- prometheus.MustNewConstMetric(descScrapeNextStart, prometheus.GaugeValue, timestamp(complete.NextCycle))
	}
	c.mu.Lock()
	if !c.completeSeen.Equal(complete.Completed) {
		c.completeSeen = complete.Completed
		c.completeMetrics = c.scrapeMetrics(scrapeComplete, *complete)
	}
	completeMetrics := c.completeMetrics
	c.mu.Unlock()
	for _, m := range completeMetrics {
		ch <- m
	}
}

// scrapeMetrics are the same for the current and the complete scrape
func (c *scrapeCollector) scrapeMetrics(scrape string, status vo.Status) []prometheus.Metric {
	gauge := func(desc *prometheus.Desc, value float64, labels ...string) prometheus.Metric {
		return prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, value, append([]string{scrape}, labels...)...)
	}

	type groupStatus struct{ group, status string }
	type histogram struct {
		count   uint64
		sum     float64
		buckets map[float64]uint64
	}
	pages := map[groupStatus]int{}
	durations := map[string]*histogram{}
	for _, r := range status.Results {
		pages[groupStatus{r.Group, strconv.Itoa(r.Code)}]++
		h, ok := durations[r.Group]
		if !ok {
			// every bucket has to be exposed, even if it is empty
			h = &histogram{buckets: make(map[float64]uint64, len(c.buckets))}
			for _, upperBound := range c.buckets {
				h.buckets[upperBound] = 0
			}
			durations[r.Group] = h
		}
		seconds := r.Duration.Seconds()
		h.count++
		h.sum += seconds
		for _, upperBound := range c.buckets {
			if seconds <= upperBound {
				h.buckets[upperBound]++
			}
		}
	}

	end := status.Completed
	if status.Running || end.IsZero() {
		end = time.Now()
	}
	metrics := []prometheus.Metric{
		gauge(descScrapeJobsOpen, float64(len(status.Jobs))),
		gauge(descScrapeStarted, timestamp(status.Started)),
		gauge(descScrapeDuration, end.Sub(status.Started).Seconds()),
	}
	if !status.Completed.IsZero() {
		metrics = append(metrics, gauge(descScrapeCompleted, timestamp(status.Completed)))
	}
	for gs, count := range pages {
		metrics = append(metrics, gauge(descScrapePages, float64(count), gs.group, gs.status))
	}
	for group, h := range durations {
		metrics = append(metrics, prometheus.MustNewConstHistogram(
			descScrapeRequestDuration, h.count, h.sum, h.buckets, scrape, group,
		))
	}
	for _, v := range aggregateValidations(status) {
		metrics = append(metrics, gauge(descValidationScore, v.averageScore(), v.group, v.path))
		for validationType, penalty := range v.averagePenalties() {
			metrics = append(metrics, gauge(descValidationPenalty, penalty, v.group, v.path, validationType))
		}
	}
	return metrics
}

func timestamp(t time.Time) float64 {
	return float64(t.UnixNano()) / float64(time.Second)
}
