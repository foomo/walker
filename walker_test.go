package walker

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/foomo/walker/config"
	"github.com/foomo/walker/htmlschema/example"
	"github.com/foomo/walker/vo"
	"github.com/stretchr/testify/assert"
)

func getExampleDir(path ...string) string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(append([]string{filepath.Dir(filename)}, path...)...)
}

func TestSortPathsByLength(t *testing.T) {
	paths := []string{"/", "/deeper", "/deeper/and/deeper"}
	sortedPaths := sortPathsByLength(paths)
	for i, p := range paths {
		assert.Equal(t, sortedPaths[len(sortedPaths)-1-i], p)
	}
}

func TestWalker(t *testing.T) {
	s := example.NewServer(getExampleDir("htmlschema", "example", "htdocs"))
	testServer := httptest.NewServer(s)
	defer testServer.Close()
	w := NewWalker()
	conf := &config.Config{
		Target: config.Target{
			BaseURL: testServer.URL,
			Paths: []string{
				"/",
			},
		},
		GroupHeader:  example.GroupHeader,
		IgnoreRobots: true,
		Concurrency:  1,
		SchemaRoot:   getExampleDir("htmlschema", "example", "schema", "groups"),
	}
	chanStatus, errWalk := w.Walk(conf, nil, nil, nil, nil)
	assert.NoError(t, errWalk)
StatusLoop:
	for {
		select {
		case status := <-chanStatus:
			type score struct {
				score   int
				penalty int
				pages   int
			}
			groupScores := map[string]*score{}
			w.Stop()
			for _, r := range status.Results {
				fmt.Println(r.Code, r.TargetURL)
				if r.ValidationReport != nil {
					groupScore, okGroupScore := groupScores[r.Group]
					if !okGroupScore {
						groupScore = &score{}
						groupScores[r.Group] = groupScore
					}
					groupScore.score += r.ValidationReport.Score
					groupScore.pages++
					for _, v := range r.ValidationReport.Validations {
						groupScore.penalty += v.Penalty
					}
				} else {
					fmt.Println("validation error", r.ValidionError)
				}
			}
			spew.Dump(groupScores)
			break StatusLoop
		case <-time.After(time.Second * 10):
			t.Log("10s")
		}
	}
}

func waitForLoop(t *testing.T, chanLoopComplete chan vo.Status, timeout time.Duration) vo.Status {
	t.Helper()
	select {
	case status := <-chanLoopComplete:
		return status
	case <-time.After(timeout):
		t.Fatal("timed out waiting for a completed scrape cycle")
	}
	return vo.Status{}
}

func scheduleTestConf(url string, interval time.Duration) *config.Config {
	return &config.Config{
		Target:       config.Target{BaseURL: url, Paths: []string{"/"}},
		IgnoreRobots: true,
		Concurrency:  2,
		Interval:     interval,
	}
}

func TestWalkerSchedule(t *testing.T) {
	interval := 2 * time.Second
	testServer := httptest.NewServer(example.NewServer(getExampleDir("htmlschema", "example", "htdocs")))
	defer testServer.Close()
	w := NewWalker()
	defer w.Stop()
	// start right after a boundary, so that the first cycle has a full interval
	now := time.Now()
	time.Sleep(cycleSlot(now, interval, 0).Add(interval + 10*time.Millisecond).Sub(now))
	chanLoopComplete, errWalk := w.Walk(scheduleTestConf(testServer.URL, interval), nil, nil, nil, nil)
	assert.NoError(t, errWalk)

	waitForLoop(t, chanLoopComplete, 2*interval)
	status := w.GetStatus()
	assert.True(t, status.Waiting, "should idle until the next boundary")
	assert.True(t, status.NextCycle.Equal(cycleSlot(status.NextCycle, interval, 0)), "next cycle must be aligned")
	assert.NotEmpty(t, status.Results, "completed results stay visible while waiting")

	waitForLoop(t, chanLoopComplete, 3*interval)
	assert.True(t, time.Now().After(status.NextCycle), "second cycle must not start before its slot")
}

func TestWalkerScheduleOverrun(t *testing.T) {
	interval := time.Second
	htdocs := example.NewServer(getExampleDir("htmlschema", "example", "htdocs"))
	testServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		time.Sleep(interval + 200*time.Millisecond)
		htdocs.ServeHTTP(rw, r)
	}))
	defer testServer.Close()
	w := NewWalker()
	defer w.Stop()
	chanLoopComplete, errWalk := w.Walk(scheduleTestConf(testServer.URL, interval), nil, nil, nil, nil)
	assert.NoError(t, errWalk)

	waitForLoop(t, chanLoopComplete, 15*time.Second)
	status := w.GetStatus()
	assert.False(t, status.Waiting, "an overrun cycle must restart immediately")
	assert.True(t, status.CycleStart.Equal(cycleSlot(status.CycleStart, interval, 0)), "restarted cycle must resync to the schedule")
	assert.True(t, status.CycleDeadline.After(time.Now().Add(-interval)))
}
