package metrics

import (
	"net/url"
	"slices"
	"strings"

	"github.com/foomo/walker/go/internal/semanticshape"
	"github.com/foomo/walker/go/internal/vo"
)

var severities = []semanticshape.Severity{semanticshape.SeverityError, semanticshape.SeverityWarn, semanticshape.SeverityInfo}

// validationAggregate sums up the schema validation reports of the pages of a
// group below a path - score and penalty are kept apart
type validationAggregate struct {
	group string
	path  string
	pages int
	// ratio is the sum of points / max of the pages with a maximum score
	ratio      float64
	ratioPages int
	penalty    int
	findings   map[semanticshape.Severity]int
}

func (v *validationAggregate) averageRatio() float64 {
	if v.ratioPages == 0 {
		return 0
	}
	return v.ratio / float64(v.ratioPages)
}

func (v *validationAggregate) averagePenalty() float64 {
	return float64(v.penalty) / float64(v.pages)
}

func (v *validationAggregate) averageFindings() map[semanticshape.Severity]float64 {
	averages := make(map[semanticshape.Severity]float64, len(severities))
	for _, s := range severities {
		averages[s] = float64(v.findings[s]) / float64(v.pages)
	}
	return averages
}

// componentAggregate sums up the instances of a component in a group
type componentAggregate struct {
	group     string
	component string
	points    int
	max       int
}

func (c *componentAggregate) ratio() float64 {
	if c.max == 0 {
		return 0
	}
	return float64(c.points) / float64(c.max)
}

// aggregateValidations sums up the schema validation reports of a status by
// group and the longest matching target path, and the components by group
func aggregateValidations(status vo.Status) ([]*validationAggregate, []*componentAggregate) {
	sortedPaths := longestFirst(status.Paths)
	type key struct{ group, path string }
	aggregates := map[key]*validationAggregate{}
	list := []*validationAggregate{}
	components := map[key]*componentAggregate{}
	componentList := []*componentAggregate{}
	for _, r := range status.Results {
		report := r.ValidationReport
		if report == nil || report.Score == nil {
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
			a = &validationAggregate{group: r.Group, path: path, findings: map[semanticshape.Severity]int{}}
			aggregates[k] = a
			list = append(list, a)
		}
		a.pages++
		if report.Score.Max > 0 {
			a.ratio += float64(report.Score.Points) / float64(report.Score.Max)
			a.ratioPages++
		}
		a.penalty += report.Penalty
		for _, f := range report.Findings {
			a.findings[f.Severity]++
		}
		for name, summary := range report.Components {
			ck := key{r.Group, name}
			c, ok := components[ck]
			if !ok {
				c = &componentAggregate{group: r.Group, component: name}
				components[ck] = c
				componentList = append(componentList, c)
			}
			c.points += summary.Points
			c.max += summary.Max
		}
	}
	return list, componentList
}

// longestFirst sorts a copy of paths by length, longest first - the first
// prefix match is the longest
func longestFirst(paths []string) []string {
	sorted := slices.Clone(paths)
	slices.SortStableFunc(sorted, func(a, b string) int { return len(b) - len(a) })
	return sorted
}
