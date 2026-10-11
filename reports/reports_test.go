package reports

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/foomo/walker/vo"
	"github.com/stretchr/testify/assert"
)

func TestReportCompletedCycle(t *testing.T) {
	started := time.Date(2026, time.October, 10, 14, 0, 0, 0, time.UTC)
	complete := &vo.Status{
		Results: map[string]vo.ScrapeResult{
			"http://example.com/": {TargetURL: "http://example.com/", Code: 200},
		},
		Started:   started,
		Completed: started.Add(10 * time.Minute),
		NextCycle: started.Add(time.Hour),
	}
	buf := &bytes.Buffer{}
	report(reportList, buf, nil, []string{statusRunning, statusComplete}, complete, nil)
	out := buf.String()

	assert.Contains(t, out, "no scrape cycle running, last cycle completed at 2026-10-10T14:10:00Z")
	assert.Contains(t, out, "next cycle at 2026-10-10T15:00:00Z")
	assert.Contains(t, out, "took 10m0s")
	assert.Equal(t, 1, strings.Count(out, "http://example.com/"), "a completed cycle must not be reported twice")
}

func TestReportNothingCompletedYet(t *testing.T) {
	running := &vo.Status{Running: true, Started: time.Now()}
	buf := &bytes.Buffer{}
	report(reportList, buf, nil, []string{statusRunning, statusComplete}, nil, running)
	out := buf.String()
	assert.Contains(t, out, "STATUS running\n")
	assert.Contains(t, out, "no scrape cycle has completed yet")
}

func TestReportSummaryPerformanceBuckets(t *testing.T) {
	results := map[string]vo.ScrapeResult{
		"a": {TargetURL: "a", Group: "g", Duration: 0, Time: time.Unix(10, 0)},
		"b": {TargetURL: "b", Group: "g", Duration: 50 * time.Millisecond, Time: time.Unix(20, 0)},
	}
	buf := &bytes.Buffer{}
	groupedBucketListStatus(buf, results)
	out := buf.String()
	// both boundary durations are counted - they used to fall through
	assert.Contains(t, out, "1 \t 50 %\t( 0s => 50ms ) awesome")
	assert.Contains(t, out, "1 \t 50 %\t( 50ms => 100ms ) great")
	assert.Contains(t, out, "=> "+time.Unix(10, 0).String()+" - "+time.Unix(20, 0).String())
}
