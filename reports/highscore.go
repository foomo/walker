package reports

import (
	"io"
	"sort"
	"time"

	"github.com/foomo/walker/vo"
)

type score struct {
	TargetURL string
	Code      int
	Duration  time.Duration
}

type scores []score

func (s scores) Len() int           { return len(s) }
func (s scores) Less(i, j int) bool { return s[i].Duration < s[j].Duration }
func (s scores) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func reportHighscore(status vo.Status, w io.Writer, filter scrapeResultFilter) {
	printh, println, _ := printers(w)
	printh("high score")
	scores := make(scores, len(status.Results))
	i := 0
	for _, r := range status.Results {
		if filter != nil && filter(r) == false {
			continue
		}
		scores[i] = score{
			Duration:  r.Duration,
			Code:      r.Code,
			TargetURL: r.TargetURL,
		}
		i++
	}
	sort.Sort(scores)
	for i, s := range scores {
		println(i, s.Code, s.TargetURL, s.Duration)
	}
}
