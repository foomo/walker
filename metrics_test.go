package walker

import (
	"strings"
	"testing"
	"time"

	"github.com/foomo/walker/htmlschema"
	"github.com/foomo/walker/vo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func testStatus(running bool, started time.Time) *vo.Status {
	status := &vo.Status{
		Results: map[string]vo.ScrapeResult{
			"http://example.com/": {TargetURL: "http://example.com/", Group: "content", Code: 200, Duration: 40 * time.Millisecond},
			"http://example.com/shop/a": {TargetURL: "http://example.com/shop/a", Group: "product", Code: 200, Duration: 150 * time.Millisecond, ValidationReport: &htmlschema.Report{
				Score:      &htmlschema.ScoreNode{Points: 10, Max: 20},
				Penalty:    4,
				Findings:   []*htmlschema.Finding{{Severity: htmlschema.SeverityWarn, Penalty: 4}},
				Components: map[string]*htmlschema.ComponentSummary{"/components/teaser": {Instances: 2, Points: 3, Max: 4}},
			}},
			"http://example.com/shop/b": {TargetURL: "http://example.com/shop/b", Group: "product", Code: 200, Duration: 2 * time.Second, ValidationReport: &htmlschema.Report{
				Score:      &htmlschema.ScoreNode{Points: 20, Max: 20},
				Components: map[string]*htmlschema.ComponentSummary{"/components/teaser": {Instances: 1, Points: 2, Max: 2}},
			}},
			"http://example.com/missing": {TargetURL: "http://example.com/missing", Group: "content", Code: 404, Duration: 10 * time.Millisecond},
		},
		Jobs:    map[string]bool{},
		Running: running,
		Started: started,
		Paths:   []string{"/", "/shop"},
	}
	if running {
		status.Jobs["http://example.com/next"] = false
	} else {
		status.Completed = started.Add(10 * time.Minute)
		status.NextCycle = started.Add(time.Hour)
	}
	return status
}

func gatherScrape(t *testing.T, complete, running *vo.Status) (*prometheus.Registry, string) {
	t.Helper()
	registry := prometheus.NewRegistry()
	registry.MustRegister(newScrapeCollector(func() (*vo.Status, *vo.Status) { return complete, running }))
	families, err := registry.Gather()
	assert.NoError(t, err)
	names := []string{}
	for _, f := range families {
		names = append(names, f.GetName())
	}
	return registry, strings.Join(names, "\n")
}

func TestScrapeCollectorCurrentAndComplete(t *testing.T) {
	started := time.Date(2026, time.October, 10, 14, 0, 0, 0, time.UTC)
	registry, _ := gatherScrape(t, testStatus(false, started), testStatus(true, started.Add(time.Hour)))

	assert.NoError(t, testutil.GatherAndCompare(registry, strings.NewReader(`
# HELP walker_scrape_pages scraped pages by group and http status code
# TYPE walker_scrape_pages gauge
walker_scrape_pages{group="content",scrape="complete",status="200"} 1
walker_scrape_pages{group="content",scrape="complete",status="404"} 1
walker_scrape_pages{group="product",scrape="complete",status="200"} 2
walker_scrape_pages{group="content",scrape="current",status="200"} 1
walker_scrape_pages{group="content",scrape="current",status="404"} 1
walker_scrape_pages{group="product",scrape="current",status="200"} 2
# HELP walker_scrape_jobs_open pages, that are queued or being scraped
# TYPE walker_scrape_jobs_open gauge
walker_scrape_jobs_open{scrape="complete"} 0
walker_scrape_jobs_open{scrape="current"} 1
# HELP walker_scrape_running 1 while a scrape cycle is in progress, 0 while waiting for the next one
# TYPE walker_scrape_running gauge
walker_scrape_running 1
# HELP walker_scrape_schema_score_ratio average share of the maximum html schema score the validated pages reached
# TYPE walker_scrape_schema_score_ratio gauge
walker_scrape_schema_score_ratio{group="product",path="/shop",scrape="complete"} 0.75
walker_scrape_schema_score_ratio{group="product",path="/shop",scrape="current"} 0.75
# HELP walker_scrape_schema_penalty average html schema penalty of the validated pages
# TYPE walker_scrape_schema_penalty gauge
walker_scrape_schema_penalty{group="product",path="/shop",scrape="complete"} 2
walker_scrape_schema_penalty{group="product",path="/shop",scrape="current"} 2
# HELP walker_scrape_schema_findings average number of html schema findings of the validated pages by severity
# TYPE walker_scrape_schema_findings gauge
walker_scrape_schema_findings{group="product",path="/shop",scrape="complete",severity="error"} 0
walker_scrape_schema_findings{group="product",path="/shop",scrape="complete",severity="info"} 0
walker_scrape_schema_findings{group="product",path="/shop",scrape="complete",severity="warn"} 0.5
walker_scrape_schema_findings{group="product",path="/shop",scrape="current",severity="error"} 0
walker_scrape_schema_findings{group="product",path="/shop",scrape="current",severity="info"} 0
walker_scrape_schema_findings{group="product",path="/shop",scrape="current",severity="warn"} 0.5
# HELP walker_scrape_schema_component_score_ratio share of the maximum html schema score the instances of a component reached
# TYPE walker_scrape_schema_component_score_ratio gauge
walker_scrape_schema_component_score_ratio{component="/components/teaser",group="product",scrape="complete"} 0.8333333333333334
walker_scrape_schema_component_score_ratio{component="/components/teaser",group="product",scrape="current"} 0.8333333333333334
# HELP walker_scrape_completed_timestamp_seconds unix timestamp of the end of the scrape cycle
# TYPE walker_scrape_completed_timestamp_seconds gauge
walker_scrape_completed_timestamp_seconds{scrape="complete"} 1.7916414e+09
`),
		"walker_scrape_pages",
		"walker_scrape_jobs_open",
		"walker_scrape_running",
		"walker_scrape_schema_score_ratio",
		"walker_scrape_schema_penalty",
		"walker_scrape_schema_findings",
		"walker_scrape_schema_component_score_ratio",
		"walker_scrape_completed_timestamp_seconds",
	))
}

func TestScrapeCollectorDurationHistogram(t *testing.T) {
	started := time.Date(2026, time.October, 10, 14, 0, 0, 0, time.UTC)
	registry, _ := gatherScrape(t, testStatus(false, started), nil)
	// empty buckets have to be exposed as well
	assert.NoError(t, testutil.GatherAndCompare(registry, strings.NewReader(`
# HELP walker_scrape_request_duration_seconds request duration including streaming of the body
# TYPE walker_scrape_request_duration_seconds histogram
walker_scrape_request_duration_seconds_bucket{group="content",scrape="complete",le="0.05"} 2
walker_scrape_request_duration_seconds_bucket{group="content",scrape="complete",le="0.1"} 2
walker_scrape_request_duration_seconds_bucket{group="content",scrape="complete",le="0.2"} 2
walker_scrape_request_duration_seconds_bucket{group="content",scrape="complete",le="0.3"} 2
walker_scrape_request_duration_seconds_bucket{group="content",scrape="complete",le="0.5"} 2
walker_scrape_request_duration_seconds_bucket{group="content",scrape="complete",le="1"} 2
walker_scrape_request_duration_seconds_bucket{group="content",scrape="complete",le="3"} 2
walker_scrape_request_duration_seconds_bucket{group="content",scrape="complete",le="5"} 2
walker_scrape_request_duration_seconds_bucket{group="content",scrape="complete",le="10"} 2
walker_scrape_request_duration_seconds_bucket{group="content",scrape="complete",le="+Inf"} 2
walker_scrape_request_duration_seconds_sum{group="content",scrape="complete"} 0.05
walker_scrape_request_duration_seconds_count{group="content",scrape="complete"} 2
walker_scrape_request_duration_seconds_bucket{group="product",scrape="complete",le="0.05"} 0
walker_scrape_request_duration_seconds_bucket{group="product",scrape="complete",le="0.1"} 0
walker_scrape_request_duration_seconds_bucket{group="product",scrape="complete",le="0.2"} 1
walker_scrape_request_duration_seconds_bucket{group="product",scrape="complete",le="0.3"} 1
walker_scrape_request_duration_seconds_bucket{group="product",scrape="complete",le="0.5"} 1
walker_scrape_request_duration_seconds_bucket{group="product",scrape="complete",le="1"} 1
walker_scrape_request_duration_seconds_bucket{group="product",scrape="complete",le="3"} 2
walker_scrape_request_duration_seconds_bucket{group="product",scrape="complete",le="5"} 2
walker_scrape_request_duration_seconds_bucket{group="product",scrape="complete",le="10"} 2
walker_scrape_request_duration_seconds_bucket{group="product",scrape="complete",le="+Inf"} 2
walker_scrape_request_duration_seconds_sum{group="product",scrape="complete"} 2.15
walker_scrape_request_duration_seconds_count{group="product",scrape="complete"} 2
`), "walker_scrape_request_duration_seconds"))
}

func TestScrapeCollectorIdle(t *testing.T) {
	started := time.Date(2026, time.October, 10, 14, 0, 0, 0, time.UTC)
	complete := testStatus(false, started)
	registry, names := gatherScrape(t, complete, nil)

	assert.Equal(t, timestamp(complete.NextCycle), gaugeValue(t, registry, "walker_scrape_next_start_timestamp_seconds"))
	assert.Equal(t, 0.0, gaugeValue(t, registry, "walker_scrape_running"))
	assert.Contains(t, names, "walker_scrape_pages")

	// nothing completed yet and nothing running
	_, names = gatherScrape(t, nil, nil)
	assert.Equal(t, "walker_scrape_running", names)
}

// gaugeValue returns the value of an unlabeled gauge, -1 if it is missing
func gaugeValue(t *testing.T, registry *prometheus.Registry, name string) float64 {
	t.Helper()
	families, err := registry.Gather()
	assert.NoError(t, err)
	for _, f := range families {
		if f.GetName() == name {
			return f.GetMetric()[0].GetGauge().GetValue()
		}
	}
	return -1
}
