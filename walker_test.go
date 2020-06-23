package walker

import (
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/foomo/walker/config"
	"github.com/foomo/walker/htmlschema/example"
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
