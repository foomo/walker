package semanticshape

import (
	"golang.org/x/net/html"
)

// TraceStatus is the state of a node in a validation trace
type TraceStatus string

const (
	TraceStatusOK     TraceStatus = "ok"
	TraceStatusInfo   TraceStatus = "info"
	TraceStatusWarn   TraceStatus = "warn"
	TraceStatusError  TraceStatus = "error"
	TraceStatusAbsent TraceStatus = "absent"
)

// TraceNode is a schema element evaluated in one context - the schema as a
// tree, for tools that show how a page was validated
type TraceNode struct {
	// Kind is page, element, selector, choice, sequence, component, ld, prop or match
	Kind   string
	Label  string
	Path   string
	Source string
	// Rules are the attributes of the schema element as written
	Rules string
	// Status is the worst severity of the node's own findings, absent if an
	// optional element was not found
	Status TraceStatus
	Points int
	Max    int
	// Penalty of the findings of the node and its subtree
	Penalty int
	// Locators of the matched page nodes, the context if nothing matched
	Locators []string
	Findings []*Finding
	Children []*TraceNode
	// Found is false if nothing matched - the locators point to the context then
	Found bool
	// Via is the wildcard the matches were found through, if a wrapper was skipped
	Via string
}

// Trace validates a page like Validate and returns the evaluated schema as a tree
func (r *Root) Trace(group string, htmlBytes []byte) (*Report, *TraceNode, error) {
	return r.validate(group, htmlBytes, true)
}

var traceKinds = map[kind]string{
	kindElement:  "element",
	kindSelector: "selector",
	kindChoice:   "choice",
	kindSequence: "sequence",
	kindComp:     "component",
	kindLD:       "ld",
	kindProp:     "prop",
}

// enter adds a trace node below the current one and makes it current, leave
// restores the parent - both do nothing while not tracing
func (v *validator) enter(kind, label, path, source, rules string, max int) *TraceNode {
	if v.trace == nil {
		return nil
	}
	n := &TraceNode{Kind: kind, Label: label, Path: path, Source: source, Rules: rules, Max: max}
	v.trace.Children = append(v.trace.Children, n)
	v.traceStack = append(v.traceStack, v.trace)
	v.trace = n
	return n
}

func (v *validator) enterElement(e *element, path string) *TraceNode {
	return v.enter(traceKinds[e.kind], e.label, joinPath(path, e.label), e.source, e.rules, e.max)
}

func (v *validator) leave(n *TraceNode) {
	if n == nil {
		return
	}
	v.trace = v.traceStack[len(v.traceStack)-1]
	v.traceStack = v.traceStack[:len(v.traceStack)-1]
}

// match records the page nodes a trace node matched
func (n *TraceNode) match(nodes ...*html.Node) {
	if n == nil {
		return
	}
	for _, node := range nodes {
		n.Locators = append(n.Locators, locator(node))
	}
	n.Found = n.Found || len(nodes) > 0
}

// via records the wildcard, if a match sits below a skipped wrapper
func (n *TraceNode) via(ctx *html.Node, skip *wildcard, nodes []*html.Node) {
	if n == nil || skip == nil {
		return
	}
	for _, node := range nodes {
		if node.Parent != ctx {
			n.Via = skip.text
			return
		}
	}
}

// context points a trace node, that matched nothing, to where it is missing
func (n *TraceNode) context(ctx *html.Node) {
	if n == nil || n.Found || ctx == nil || ctx.Type != html.ElementNode {
		return
	}
	n.Locators = []string{locator(ctx)}
}

func (n *TraceNode) points(points int) {
	if n != nil {
		n.Points = points
	}
}

// finish computes status and penalty of the trace below n
func (n *TraceNode) finish() int {
	worst := Severity("")
	n.Penalty = 0
	for _, f := range n.Findings {
		n.Penalty += f.Penalty
		if f.Severity.rank() > worst.rank() {
			worst = f.Severity
		}
	}
	for _, c := range n.Children {
		n.Penalty += c.finish()
	}
	switch {
	case worst != "":
		n.Status = TraceStatus(worst)
	case !n.Found && (n.Kind == "element" || n.Kind == "selector" || n.Kind == "component" || n.Kind == "ld" || n.Kind == "prop"):
		n.Status = TraceStatusAbsent
	default:
		n.Status = TraceStatusOK
	}
	return n.Penalty
}
