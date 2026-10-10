package walker

import "github.com/prometheus/client_golang/prometheus"

type cycleMetrics struct {
	running        prometheus.Gauge
	duration       prometheus.Gauge
	overruns       prometheus.Counter
	nextStartEpoch prometheus.Gauge
}

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
	cycle cycleMetrics,
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

	cycle = cycleMetrics{
		running: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "walker_scrape_cycle_running",
			Help: "1 while a scrape cycle is in progress, 0 while waiting for the next one",
		}),
		duration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "walker_scrape_cycle_duration_seconds",
			Help: "duration of the last completed scrape cycle",
		}),
		overruns: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "walker_scrape_cycle_overruns_total",
			Help: "number of scrape cycles, that took longer than the configured interval",
		}),
		nextStartEpoch: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "walker_scrape_cycle_next_start_timestamp_seconds",
			Help: "unix timestamp of the next scheduled scrape cycle start",
		}),
	}

	for _, c := range []prometheus.Collector{
		cycle.running,
		cycle.duration,
		cycle.overruns,
		cycle.nextStartEpoch,
		summaryVec,
		counterVec,
		totalCounter,
		counterVecStatus,
		progressGaugeOpen,
		progressGaugeComplete,
		schemaValidationScoreVec,
		schemaValidationPenaltyVec,
	} {
		// only the first walker in a process exports metrics - previously
		// a second walker panicked here
		if err := prometheus.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				panic(err)
			}
		}
	}
	return
}
