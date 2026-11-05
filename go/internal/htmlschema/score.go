package htmlschema

import (
	"sort"
)

// computeMax sets the maximum score of e and its subtree, see SPEC.md 9.2.
// bounded is false if the maximum is infinite, which is a lint error.
func (c *compiler) computeMax(e *element) (max int, bounded bool) {
	switch e.kind {
	case kindChoice:
		max, bounded = c.choiceMax(e)
	case kindSequence:
		bounded = true
		for _, child := range e.children {
			m, b := c.computeMax(child)
			max += m
			bounded = bounded && b
		}
	default:
		perNode, perNodeBounded := e.score, e.scorePer == "" || e.score == 0
		if e.kind == kindComp {
			perNode, perNodeBounded = c.componentMax(e.comp), true
		}
		for _, child := range e.children {
			m, b := c.computeMax(child)
			perNode += m
			perNodeBounded = perNodeBounded && b
		}
		bounded = perNodeBounded
		e.nodeMax = perNode
		if !perNodeBounded || (e.scoreMax != unbounded && perNode > e.scoreMax) {
			e.nodeMax = e.scoreMax
		}
		switch {
		case e.occ.max == unbounded && (perNode > 0 || !perNodeBounded):
			bounded = false
		case e.occ.max != unbounded:
			max = e.occ.max * perNode
		}
	}
	if e.scoreMax != unbounded {
		if !bounded || max > e.scoreMax {
			max = e.scoreMax
		}
		bounded = true
	}
	if !bounded {
		c.errs = append(c.errs, LintError{File: sourceFile(e.source), Line: sourceLine(e.source), Message: "the maximum score is unbounded, add score-max"})
		max = 0
	}
	e.max = max
	return max, true
}

// choiceMax is the best combination of instances: first the minimum of every
// component, then the most valuable ones up to their own and the choice's max
func (c *compiler) choiceMax(e *element) (max int, bounded bool) {
	type item struct {
		use   *element
		per   int
		taken int
	}
	items := make([]*item, len(e.children))
	for i, use := range e.children {
		// a use on its own may be unbounded, only the combination counts
		items[i] = &item{use: use, per: c.componentMax(use.comp)}
	}
	slots := e.occ.max
	take := func(it *item, want int) bool {
		if want == unbounded && slots == unbounded {
			return it.per == 0
		}
		n := want
		if n == unbounded || (slots != unbounded && n > slots) {
			n = slots
		}
		it.taken += n
		if slots != unbounded {
			slots -= n
		}
		return true
	}
	bounded = true
	for _, it := range items {
		take(it, it.use.occ.min)
	}
	sorted := append([]*item{}, items...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].per > sorted[j].per })
	for _, it := range sorted {
		remaining := unbounded
		if it.use.occ.max != unbounded {
			remaining = it.use.occ.max - it.taken
		}
		if !take(it, remaining) {
			bounded = false
		}
	}
	for _, it := range items {
		points := it.taken * it.per
		if it.use.scoreMax != unbounded && points > it.use.scoreMax {
			points = it.use.scoreMax
		}
		max += points
	}
	return max, bounded
}

func (c *compiler) componentMax(comp *component) int {
	switch comp.maxState {
	case 2:
		return comp.max
	case 1:
		// a component containing itself - its own maximum can not count twice
		return 0
	}
	comp.maxState = 1
	max := 0
	for _, e := range comp.body {
		m, _ := c.computeMax(e)
		max += m
	}
	comp.max = max
	comp.maxState = 2
	return max
}
