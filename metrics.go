package walker

import "github.com/prometheus/client_golang/prometheus"

func setupMetrics() (
	summaryVec *prometheus.SummaryVec,
	counterVec *prometheus.CounterVec,
	totalCounter prometheus.Counter,
	progressGaugeOpen prometheus.Gauge,
	progressGaugeComplete prometheus.Gauge,
	counterVecStatus *prometheus.CounterVec,
) {

	const prometheusLabelGroup = "group"
	const prometheusLabelStatus = "status"

	summaryVec = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "walker_scrape_durations_seconds",
			Help:       "scrape duration whole request time including streaming of body",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{prometheusLabelGroup},
	)

	counterVec = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "walker_scrape_running_total",
			Help: "Number of scrapes in scan.",
		},
		[]string{prometheusLabelGroup, prometheusLabelStatus},
	)

	totalCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "walker_scrape_counter_total",
		Help: "number of scrapes since start of walker",
	})

	progressGaugeOpen = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "walker_progress_gauge_open",
			Help: "progress open to scrape",
		},
	)

	progressGaugeComplete = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "walker_progress_gauge_complete",
			Help: "progress complete scrapes",
		},
	)

	counterVecStatus = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "walker_progress_status_code_total",
		Help: "status codes for running scrape",
	}, []string{prometheusLabelStatus})

	prometheus.MustRegister(
		summaryVec,
		counterVec,
		totalCounter,
		progressGaugeComplete,
		progressGaugeOpen,
		counterVecStatus,
	)

	return
}
