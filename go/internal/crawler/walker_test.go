package crawler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/foomo/walker/go/internal/config"
	"github.com/foomo/walker/go/internal/demoshop"
	"github.com/foomo/walker/go/internal/vo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// demoHandler serves the demo shop, as compiled into the test binary
func demoHandler(t *testing.T) http.Handler {
	t.Helper()
	handler, err := demoshop.NewHandler(demoshop.EmbeddedFiles(), false)
	require.NoError(t, err)
	return handler
}

func TestSortPathsByLength(t *testing.T) {
	paths := []string{"/", "/deeper", "/deeper/and/deeper"}
	sortedPaths := sortPathsByLength(paths)
	for i, p := range paths {
		assert.Equal(t, sortedPaths[len(sortedPaths)-1-i], p)
	}
}

func TestWalker(t *testing.T) {
	testServer := httptest.NewServer(demoHandler(t))
	defer testServer.Close()
	w := New(nil)
	conf := &config.Config{
		Target: config.Target{
			BaseURL: testServer.URL,
			Paths: []string{
				"/",
			},
		},
		GroupHeader:  demoshop.GroupHeader,
		IgnoreRobots: true,
		Concurrency:  1,
		ShapeRoot:    demoshop.ShapesDir(),
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
					groupScore.score += r.ValidationReport.Score.Points
					groupScore.penalty += r.ValidationReport.Penalty
					groupScore.pages++
				} else {
					fmt.Println("validation error", r.ValidationError)
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
		Target: config.Target{BaseURL: url, Paths: []string{"/"}},
		// only the home page and its css - the overrun test delays every request
		Ignore:       []string{"/category", "/product", "/images"},
		IgnoreRobots: true,
		Concurrency:  2,
		Interval:     interval,
	}
}

func TestWalkerSchedule(t *testing.T) {
	interval := 2 * time.Second
	testServer := httptest.NewServer(demoHandler(t))
	defer testServer.Close()
	w := New(nil)
	defer w.Stop()
	// start right after a boundary, so that the first cycle has a full interval
	now := time.Now()
	time.Sleep(cycleSlot(now, interval, 0).Add(interval + 10*time.Millisecond).Sub(now))
	chanLoopComplete, errWalk := w.Walk(scheduleTestConf(testServer.URL, interval), nil, nil, nil, nil)
	assert.NoError(t, errWalk)

	completed := waitForLoop(t, chanLoopComplete, 2*interval)
	assert.False(t, completed.Running)
	assert.False(t, completed.Completed.IsZero())
	status := w.GetStatus()
	assert.False(t, status.Running, "should idle until the next boundary")
	assert.True(t, status.NextCycle.Equal(cycleSlot(status.NextCycle, interval, 0)), "next cycle must be aligned")
	assert.NotEmpty(t, status.Results, "completed results stay visible while waiting")

	complete, running := w.GetStatuses()
	assert.Nil(t, running, "no running status while waiting")
	if assert.NotNil(t, complete) {
		assert.Equal(t, status.NextCycle, complete.NextCycle)
	}
	// a completed cycle is frozen - speeds must not decay while waiting
	time.Sleep(1100 * time.Millisecond)
	assert.Equal(t, status.ScrapeTotalSeconds, w.GetStatus().ScrapeTotalSeconds)

	waitForLoop(t, chanLoopComplete, 3*interval)
	assert.True(t, time.Now().After(status.NextCycle), "second cycle must not start before its slot")
}

func TestWalkerScheduleOverrun(t *testing.T) {
	interval := time.Second
	shop := demoHandler(t)
	testServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		time.Sleep(interval + 200*time.Millisecond)
		shop.ServeHTTP(rw, r)
	}))
	defer testServer.Close()
	w := New(nil)
	defer w.Stop()
	chanLoopComplete, errWalk := w.Walk(scheduleTestConf(testServer.URL, interval), nil, nil, nil, nil)
	assert.NoError(t, errWalk)

	waitForLoop(t, chanLoopComplete, 15*time.Second)
	status := w.GetStatus()
	assert.True(t, status.Running, "an overrun cycle must restart immediately")
	assert.True(t, status.CycleStart.Equal(cycleSlot(status.CycleStart, interval, 0)), "restarted cycle must resync to the schedule")
	assert.True(t, status.CycleDeadline.After(time.Now().Add(-interval)))
}

func TestWalkerRestart(t *testing.T) {
	testServer := httptest.NewServer(demoHandler(t))
	defer testServer.Close()
	w := New(nil)
	defer w.Stop()
	assert.False(t, w.Restart(), "nothing to restart before a walk")
	// scheduled once an hour - without a restart there is no second cycle
	chanLoopComplete, errWalk := w.Walk(scheduleTestConf(testServer.URL, time.Hour), nil, nil, nil, nil)
	assert.NoError(t, errWalk)
	first := waitForLoop(t, chanLoopComplete, 10*time.Second)

	assert.True(t, w.Restart())
	assert.True(t, w.GetStatus().Running, "the restart starts a cycle right away")
	second := waitForLoop(t, chanLoopComplete, 10*time.Second)
	assert.True(t, second.Started.After(first.Started))
	assert.Equal(t, len(first.Results), len(second.Results))

	// aborting a running cycle: the results of its requests in flight are dropped
	assert.True(t, w.Restart())
	assert.True(t, w.Restart())
	third := waitForLoop(t, chanLoopComplete, 10*time.Second)
	assert.Equal(t, len(first.Results), len(third.Results))
}
