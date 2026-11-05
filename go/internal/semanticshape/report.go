package semanticshape

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
)

// Finding is a rule a page breaks
type Finding struct {
	Severity Severity
	// Penalty is the weight of the finding, it is never subtracted from the score
	Penalty int
	Rule    string // "occurrence", "min-length", "attr:url", "component-not-allowed", …
	// Path is the schema path with page positions, stable across layout changes
	Path string
	// Locator is a CSS path to the page node, for document.querySelector
	Locator string
	Message string
	// Component is the path of the innermost component instance, "" on page level
	Component string
	// Source is the schema file and line of the rule
	Source string
}

// ScoreNode is the score of a schema element, for drill down
type ScoreNode struct {
	Path     string
	Points   int
	Max      int
	Children []*ScoreNode
}

// ComponentSummary sums up the instances of one component on a page
type ComponentSummary struct {
	Instances int
	Points    int
	Max       int
	Penalty   int
}

// Report of a validated page. Score and penalty are separate measures and
// never combined.
type Report struct {
	Group      string
	Schema     string
	Findings   []*Finding
	Score      *ScoreNode
	Penalty    int
	Components map[string]*ComponentSummary
}

// Percent of the maximum score the page reached
func (r *Report) Percent() float64 {
	if r.Score == nil || r.Score.Max == 0 {
		return 0
	}
	return 100 * float64(r.Score.Points) / float64(r.Score.Max)
}

// Count the findings with severity s
func (r *Report) Count(s Severity) (count int) {
	for _, f := range r.Findings {
		if f.Severity == s {
			count++
		}
	}
	return count
}

func percent(points, max int) string {
	if max == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(points)/float64(max))
}

// Print the report as text
func (r *Report) Print(w io.Writer) {
	fmt.Fprintf(w, "schema %s (group %s)\n", r.Schema, r.Group)
	fmt.Fprintf(w, "score %d/%d (%s), penalty %d, findings: %d error, %d warn, %d info\n",
		r.Score.Points, r.Score.Max, percent(r.Score.Points, r.Score.Max), r.Penalty,
		r.Count(SeverityError), r.Count(SeverityWarn), r.Count(SeverityInfo))
	if len(r.Findings) > 0 {
		fmt.Fprintln(w)
		tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		for _, f := range r.Findings {
			fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\n", f.Severity, f.Penalty, f.Rule, f.Path, f.Message)
			fmt.Fprintf(tw, "\t\t\t%s\t%s\n", f.Locator, f.Source)
		}
		tw.Flush()
	}
	if len(r.Components) > 0 {
		fmt.Fprintln(w)
		paths := make([]string, 0, len(r.Components))
		for p := range r.Components {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		for _, p := range paths {
			c := r.Components[p]
			fmt.Fprintf(tw, "%s\t%d instances\t%d/%d (%s)\tpenalty %d\n", p, c.Instances, c.Points, c.Max, percent(c.Points, c.Max), c.Penalty)
		}
		tw.Flush()
	}
	fmt.Fprintln(w)
	var printScore func(s *ScoreNode, depth int)
	printScore = func(s *ScoreNode, depth int) {
		fmt.Fprintf(w, "%s%s %d/%d\n", strings.Repeat("  ", depth), s.Path, s.Points, s.Max)
		for _, c := range s.Children {
			printScore(c, depth+1)
		}
	}
	for _, s := range r.Score.Children {
		printScore(s, 0)
	}
}
