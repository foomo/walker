package semanticshape

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// Validate a page of a group: groups/<group>.semanticshape.html, or default.semanticshape.html if the
// group has no shape
func (r *Root) Validate(group string, htmlBytes []byte) (*Report, error) {
	report, _, err := r.validate(group, htmlBytes, false)
	return report, err
}

func (r *Root) validate(group string, htmlBytes []byte, trace bool) (*Report, *TraceNode, error) {
	s, err := r.shapeFor(group)
	if err != nil {
		return nil, nil, err
	}
	p, err := newPage(htmlBytes, r, s.settings)
	if err != nil {
		return nil, nil, err
	}
	v := &validator{
		shape:  s,
		page:   p,
		report: &Report{Group: group, Shape: s.path, Components: map[string]*ComponentSummary{}},
	}
	var root *TraceNode
	if trace {
		root = &TraceNode{Kind: "page", Label: s.path, Source: strings.TrimPrefix(s.path, "/") + FileSuffix, Max: s.max, Found: true}
		v.trace = root
	}
	for _, inst := range p.ambiguous {
		paths := make([]string, len(inst.ambiguous))
		for i, comp := range inst.ambiguous {
			paths[i] = comp.path
		}
		v.add(scope{}, SeverityError, v.penaltyFor(SeverityError, -1), "component-ambiguous", "", inst.node, "",
			"matches the selectors of "+strings.Join(paths, ", ")+" - narrow a selector or add a marker")
	}
	points, scores := v.validateElements(p.doc, s.elements, "", false, scope{})
	v.report.Score = &ScoreNode{Points: points, Max: s.max, Children: scores}
	for _, f := range v.report.Findings {
		v.report.Penalty += f.Penalty
		if f.Component != "" {
			v.summary(f.Component).Penalty += f.Penalty
		}
	}
	if root != nil {
		root.Points = points
		root.finish()
	}
	return v.report, root, nil
}

type validator struct {
	shape  *shape
	page   *page
	report *Report
	// trace is the current trace node, nil while not tracing
	trace      *TraceNode
	traceStack []*TraceNode
}

// scope of the validation: the innermost component instance
type scope struct {
	component string
	ceiling   Severity
	ldRoot    *html.Node // nil = page wide
}

func (v *validator) add(sc scope, sev Severity, penalty int, rule, path string, n *html.Node, source, message string) {
	f := &Finding{
		Severity:  capSeverity(sev, sc.ceiling),
		Penalty:   penalty,
		Rule:      rule,
		Path:      path,
		Message:   message,
		Component: sc.component,
		Source:    source,
	}
	if n != nil {
		f.Locator = locator(n)
	}
	v.report.Findings = append(v.report.Findings, f)
	if v.trace != nil {
		v.trace.Findings = append(v.trace.Findings, f)
	}
}

// penaltyFor is the explicit penalty or the default of the severity
func (v *validator) penaltyFor(sev Severity, explicit int) int {
	if explicit >= 0 {
		return explicit
	}
	return v.shape.settings.penalties[sev]
}

func (v *validator) summary(path string) *ComponentSummary {
	s, ok := v.report.Components[path]
	if !ok {
		s = &ComponentSummary{}
		v.report.Components[path] = s
	}
	return s
}

func joinPath(path, segment string) string {
	if path == "" {
		return segment
	}
	return path + "/" + segment
}

func indexed(label string, i, count int) string {
	if count > 1 {
		return fmt.Sprintf("%s[%d]", label, i+1)
	}
	return label
}

func (v *validator) validateElements(ctx *html.Node, elements []*element, path string, open bool, sc scope) (points int, scores []*ScoreNode) {
	componentContext := false
	for _, e := range elements {
		var (
			p int
			s *ScoreNode
		)
		switch e.kind {
		case kindChoice, kindSequence:
			componentContext = true
			p, s = v.validateContentModel(ctx, e, path, sc)
		case kindLD:
			p, s = v.validateLD(ctx, e, path, sc)
		default:
			componentContext = componentContext || e.kind == kindComp
			p, s = v.validateMatches(ctx, e, path, sc)
		}
		points += p
		if s.Max > 0 {
			scores = append(scores, s)
		}
	}
	if componentContext {
		v.checkClosed(ctx, elements, path, open, sc)
	}
	return points, scores
}

// checkOccurrence reports missing nodes and returns the nodes that count -
// the first max ones. Surplus nodes are reported by reportSurplus.
func (v *validator) checkOccurrence(ctx *html.Node, e *element, nodes []*html.Node, path string, sc scope, rule string) []*html.Node {
	n := len(nodes)
	if e.occ.max != unbounded && n > e.occ.max {
		return nodes[:e.occ.max]
	}
	if n < e.occ.min {
		v.add(sc, e.severity, v.charge(e, v.penaltyFor(e.severity, e.penalty)*(e.occ.min-n), nil), rule, joinPath(path, e.label), ctx, e.source,
			fmt.Sprintf("got %d, expected %s", n, e.occ))
	}
	return nodes
}

// reportSurplus reports every node after the first max ones, each in a trace
// node of its own - the finding belongs to that node, not to the ones that count
func (v *validator) reportSurplus(e *element, nodes []*html.Node, path string, sc scope, rule string) {
	n := len(nodes)
	if e.occ.max == unbounded || n <= e.occ.max {
		return
	}
	budget := e.penaltyMax
	message := fmt.Sprintf("got %d, expected %s", n, e.occ)
	for i := e.occ.max; i < n; i++ {
		segment := indexed(e.label, i, n)
		mt := v.enter("match", segment, joinPath(path, segment), e.source, "", 0)
		mt.match(nodes[i])
		v.add(sc, e.severity, v.charge(e, v.penaltyFor(e.severity, e.penalty), &budget), rule, joinPath(path, segment), nodes[i], e.source, message)
		v.leave(mt)
	}
}

// charge caps a penalty by penalty-max - budget is what is left of it over
// several findings, nil for a single one
func (v *validator) charge(e *element, penalty int, budget *int) int {
	if e.penaltyMax == unbounded {
		return penalty
	}
	if budget == nil {
		return min(penalty, e.penaltyMax)
	}
	if penalty > *budget {
		penalty = *budget
	}
	*budget -= penalty
	return penalty
}

func (v *validator) validateMatches(ctx *html.Node, e *element, path string, sc scope) (int, *ScoreNode) {
	nodes := v.page.candidates(ctx, e)
	tn := v.enterElement(e, path)
	defer v.leave(tn)
	counted := v.checkOccurrence(ctx, e, nodes, path, sc, "occurrence")
	tn.match(nodes...)
	tn.context(ctx)
	if e.kind != kindSelector {
		tn.via(ctx, e.skip, nodes)
	}
	score := &ScoreNode{Path: joinPath(path, e.label), Max: e.max}
	total := 0
	for i, n := range counted {
		nodePath := joinPath(path, indexed(e.label, i, len(nodes)))
		if e.kind == kindComp {
			p, s := v.validateInstance(n, e, nodePath, sc)
			total += p
			score.Children = append(score.Children, s)
			continue
		}
		// with more than one match every match gets its own trace node
		mt := tn
		if len(nodes) > 1 {
			mt = v.enter("match", indexed(e.label, i, len(nodes)), nodePath, e.source, "", e.nodeMax)
			mt.match(n)
		}
		points := 0
		if v.checkNode(n, e, nodePath, sc) {
			points = v.ownPoints(n, e)
		}
		p, s := v.validateElements(n, e.children, nodePath, e.open, sc)
		points += p
		total += points
		score.Children = append(score.Children, s...)
		if mt != tn {
			mt.points(points)
			v.leave(mt)
		}
	}
	v.reportSurplus(e, nodes, path, sc, "occurrence")
	if e.scoreMax != unbounded && total > e.scoreMax {
		total = e.scoreMax
	}
	score.Points = total
	tn.points(total)
	return total, score
}

// checkNode checks the text and attribute rules of a node, true if all pass
func (v *validator) checkNode(n *html.Node, e *element, path string, sc scope) bool {
	ok := true
	if !e.text.empty() {
		for _, f := range e.text.check(v.page.text(n)) {
			v.add(sc, e.severity, v.penaltyFor(e.severity, e.penalty), f.rule, path, n, e.source, f.message)
			ok = false
		}
	}
	for _, ar := range e.attrRules {
		val, _ := attr(n, ar.attr)
		failed, messages := ar.failures(val)
		sev, penalty := e.severity, e.penalty
		if ar.severity != "" {
			sev = ar.severity
		}
		if ar.penalty >= 0 {
			penalty = ar.penalty
		}
		for i, check := range failed {
			v.add(sc, sev, v.penaltyFor(sev, penalty), "attr:"+check.name, path+"@"+ar.attr, n, e.source, messages[i])
			ok = false
		}
	}
	return ok
}

func (v *validator) ownPoints(n *html.Node, e *element) int {
	switch e.scorePer {
	case "words":
		return e.score * (len(strings.Fields(v.page.text(n))) / e.scoreUnit)
	case "chars":
		return e.score * (utf8.RuneCountInString(v.page.text(n)) / e.scoreUnit)
	}
	return e.score
}

func (v *validator) validateInstance(n *html.Node, use *element, path string, sc scope) (int, *ScoreNode) {
	tn := v.enter("component", path[strings.LastIndex(path, "/")+1:], path, use.source, use.rules, use.comp.max)
	defer v.leave(tn)
	tn.match(n)
	inner := scope{component: use.comp.path, ceiling: sc.ceiling, ldRoot: n}
	if use.severitySet {
		inner.ceiling = capSeverity(use.severity, sc.ceiling)
	}
	points, scores := v.validateElements(n, use.comp.body, path, use.open || use.comp.open, inner)
	summary := v.summary(use.comp.path)
	summary.Instances++
	summary.Points += points
	summary.Max += use.comp.max
	tn.points(points)
	return points, &ScoreNode{Path: path, Points: points, Max: use.comp.max, Children: scores}
}

// validateContentModel validates a val:choice or val:sequence
func (v *validator) validateContentModel(ctx *html.Node, e *element, path string, sc scope) (int, *ScoreNode) {
	type found struct {
		node *html.Node
		use  int
		path string
	}
	tn := v.enterElement(e, path)
	defer v.leave(tn)
	var all []found
	counted := map[*html.Node]bool{}
	for i, use := range e.children {
		nodes := v.page.candidates(ctx, use)
		for _, n := range v.checkOccurrence(ctx, use, nodes, path, sc, "occurrence") {
			counted[n] = true
		}
		v.reportSurplus(use, nodes, path, sc, "occurrence")
		for j, n := range nodes {
			all = append(all, found{node: n, use: i, path: joinPath(path, indexed(use.label, j, len(nodes)))})
		}
	}
	order := v.page.order
	sort.SliceStable(all, func(i, j int) bool { return order[all[i].node] < order[all[j].node] })
	nodes := make([]*html.Node, len(all))
	for i, f := range all {
		nodes[i] = f.node
	}
	tn.match(nodes...)
	tn.context(ctx)
	if len(e.children) > 0 {
		tn.via(ctx, e.children[0].skip, nodes)
	}
	inTotal := map[*html.Node]bool{}
	for _, n := range v.checkOccurrence(ctx, e, nodes, path, sc, "occurrence") {
		inTotal[n] = true
	}
	v.reportSurplus(e, nodes, path, sc, "occurrence")
	if e.kind == kindSequence {
		last := -1
		for _, f := range all {
			if f.use < last {
				v.add(sc, e.severity, v.penaltyFor(e.severity, e.penalty), "component-order", f.path, f.node, e.source,
					fmt.Sprintf("%s comes after %s", e.children[f.use].label, e.children[last].label))
				continue
			}
			last = f.use
		}
	}
	score := &ScoreNode{Path: joinPath(path, e.label), Max: e.max}
	usePoints := make([]int, len(e.children))
	for _, f := range all {
		if !counted[f.node] || !inTotal[f.node] {
			continue
		}
		p, s := v.validateInstance(f.node, e.children[f.use], f.path, sc)
		usePoints[f.use] += p
		score.Children = append(score.Children, s)
	}
	total := 0
	for i, use := range e.children {
		p := usePoints[i]
		if use.scoreMax != unbounded && p > use.scoreMax {
			p = use.scoreMax
		}
		total += p
	}
	if e.scoreMax != unbounded && total > e.scoreMax {
		total = e.scoreMax
	}
	score.Points = total
	tn.points(total)
	return total, score
}

// checkClosed reports component instances in a component context that it does not use
func (v *validator) checkClosed(ctx *html.Node, elements []*element, path string, open bool, sc scope) {
	allowed := map[*component]bool{}
	var (
		skip   *wildcard
		source string
	)
	for _, e := range elements {
		var uses []*element
		switch e.kind {
		case kindComp:
			uses = []*element{e}
		case kindChoice, kindSequence:
			uses = e.children
		}
		for _, use := range uses {
			allowed[use.comp] = true
			skip, source = use.skip, use.source
		}
	}
	instances := v.page.instancesIn(ctx, skip)
	counts := map[string]int{}
	for _, inst := range instances {
		if inst.comp != nil {
			counts[inst.comp.label]++
		}
	}
	seen := map[string]int{}
	for _, inst := range instances {
		if inst.comp == nil || allowed[inst.comp] {
			continue
		}
		l := inst.comp.label
		segment := indexed(l, seen[l], counts[l])
		p := joinPath(path, segment)
		seen[l]++
		tn := v.enter("component", segment, p, source, "", 0)
		tn.match(inst.node)
		sev := SeverityError
		if open {
			sev = SeverityInfo
		}
		v.add(sc, sev, v.penaltyFor(sev, -1), "component-not-allowed", p, inst.node, source, l+" is not used in this context")
		v.leave(tn)
	}
}
