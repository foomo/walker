package walker

import (
	"net/url"
	"strings"

	"github.com/foomo/walker/vo"
)

type validationAggregate struct {
	group     string
	path      string
	pages     int
	score     int
	penalties map[string]int
}

func (v *validationAggregate) averageScore() float64 {
	return float64(v.score) / float64(v.pages)
}

func (v *validationAggregate) averagePenalties() map[string]float64 {
	averages := make(map[string]float64, len(v.penalties))
	for validationType, penalty := range v.penalties {
		averages[validationType] = float64(penalty) / float64(v.pages)
	}
	return averages
}

// aggregateValidations sums up the schema validation reports of a status by
// group and the longest matching target path
func aggregateValidations(status vo.Status) []*validationAggregate {
	sortedPaths := sortPathsByLength(status.Paths)
	type key struct{ group, path string }
	aggregates := map[key]*validationAggregate{}
	list := []*validationAggregate{}
	for _, r := range status.Results {
		if r.ValidationReport == nil {
			continue
		}
		u, errParse := url.Parse(r.TargetURL)
		if errParse != nil {
			continue
		}
		path := "/"
		for _, p := range sortedPaths {
			if strings.HasPrefix(u.Path, p) {
				path = p
				break
			}
		}
		k := key{r.Group, path}
		a, ok := aggregates[k]
		if !ok {
			a = &validationAggregate{group: r.Group, path: path, penalties: map[string]int{}}
			aggregates[k] = a
			list = append(list, a)
		}
		a.pages++
		a.score += r.ValidationReport.Score
		for _, validation := range r.ValidationReport.Validations {
			a.penalties[string(validation.Type)] += validation.Penalty
		}
	}
	return list
}
