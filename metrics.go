package walker

import "github.com/prometheus/client_golang/prometheus"

type trackValidationScore func(group, path string, score int)
type trackValidationPenalty func(group, path, validationType string, score int)

func setupMetrics() (
	summaryVec *prometheus.SummaryVec,
	counterVec *prometheus.CounterVec,
	totalCounter prometheus.Counter,
	progressGaugeOpen prometheus.Gauge,
	progressGaugeComplete prometheus.Gauge,
	counterVecStatus *prometheus.CounterVec,
	trackValidationScore trackValidationScore,
	trackValidationPenalty trackValidationPenalty,
) {

	const (
		prometheusLabelGroup          = "group"
		prometheusLabelStatus         = "status"
		prometheusLabelPath           = "path"
		prometheusLabelValidationType = "type"
	)

	summaryVec = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "walker_scrape_durations_seconds",
			Help:       "scrape duration whole request time including streaming of body",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{prometheusLabelGroup},
	)

	schemaValidationScoreVec := prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "walker_validation_score",
			Help:       "html schema score for groups in paths",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{prometheusLabelGroup, prometheusLabelPath},
	)
	trackValidationScore = func(group, path string, score int) {
		schemaValidationScoreVec.With(prometheus.Labels{
			prometheusLabelGroup: group,
			prometheusLabelPath:  path,
		}).Observe(float64(score))
	}

	schemaValidationPenaltyVec := prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "walker_validation_penalty",
			Help:       "html schema score for groups and validation types in paths",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{prometheusLabelGroup, prometheusLabelPath, prometheusLabelValidationType},
	)
	trackValidationPenalty = func(group, path, validationType string, score int) {
		schemaValidationPenaltyVec.With(prometheus.Labels{
			prometheusLabelGroup:          group,
			prometheusLabelPath:           path,
			prometheusLabelValidationType: validationType,
		}).Observe(float64(score))
	}

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
		counterVecStatus,
		progressGaugeOpen,
		progressGaugeComplete,
		schemaValidationScoreVec,
		schemaValidationPenaltyVec,
	)
	return
}
