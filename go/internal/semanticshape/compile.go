package semanticshape

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
)

type kind int

const (
	kindElement kind = iota
	kindSelector
	kindChoice
	kindSequence
	kindComp
	kindLD
	kindProp
)

type literal struct {
	key string
	val string // "*" = present and not empty
}

// element is a compiled schema element
type element struct {
	kind      kind
	name      string // tag name of kindElement
	label     string // path segment in findings
	literals  []literal
	occ       occurrence
	text      textRules
	attrRules []*attrRule
	value     valueRules // kindProp
	score     int
	scorePer  string // "", "words" or "chars"
	scoreUnit int
	scoreMax  int // unbounded = no cap
	severity  Severity
	// severitySet marks an explicit severity, on a kindComp it is a ceiling for the instances
	severitySet bool
	penalty     int // -1 = default by severity
	penaltyMax  int // unbounded = no cap
	open        bool
	// every makes a val:prop check every array element, not just one
	every bool
	// skip is the wildcard scope this element is searched with, nil = direct children only
	skip     *wildcard
	selector cascadia.Selector // kindSelector
	comp     *component        // kindComp
	ldType   string            // kindLD
	propPath []string          // kindProp
	children []*element
	source   string
	max      int // maximum score, computed after compiling
	nodeMax  int // maximum score of one matched node
	rules    string
}

// wildcard is the scope of a val:wildcard - the wrappers to skip
type wildcard struct {
	match cascadia.Selector
	text  string
}

// component is a file with a val:component root
type component struct {
	path     string // root relative, without extension: /components/teaser
	label    string
	marker   string
	selector cascadia.Selector
	node     *rawNode
	body     []*element
	open     bool
	compiled bool
	max      int
	maxState int // 0 not computed, 1 computing, 2 done
}

type settings struct {
	exclude         cascadia.Selector
	markerAttribute string
	penalties       map[Severity]int
}

func defaultSettings() settings {
	return settings{
		markerAttribute: "data-component",
		penalties:       map[Severity]int{SeverityError: 10, SeverityWarn: 3, SeverityInfo: 0},
	}
}

// schema is a compiled group schema or default.shape.html
type schema struct {
	path     string
	settings settings
	elements []*element
	max      int
}

type compiler struct {
	root         *Root
	errs         LintErrors
	stack        []string // include / comp chain for cycle detection
	settings     *settings
	settingsNode *rawNode
}

func (c *compiler) errorf(n *rawNode, format string, args ...any) {
	c.errs = append(c.errs, LintError{File: n.file, Line: n.line, Message: fmt.Sprintf(format, args...)})
}

// rule attributes, written with val: on html elements and without on val:* elements
var (
	occurrenceKeys = []string{"min", "max", "count", "optional", "forbidden"}
	textKeys       = []string{"min-length", "max-length", "min-words", "max-words"}
	scoreKeys      = []string{"score", "score-per", "score-max"}
	findingKeys    = []string{"severity", "penalty", "penalty-max"}
)

func keySet(groups ...[]string) map[string]bool {
	set := map[string]bool{}
	for _, g := range groups {
		for _, k := range g {
			set[k] = true
		}
	}
	return set
}

var allowedRules = map[kind]map[string]bool{
	kindElement:  keySet(occurrenceKeys, textKeys, scoreKeys, findingKeys, []string{"attr", "attr-*", "open"}),
	kindSelector: keySet(occurrenceKeys, textKeys, scoreKeys, findingKeys, []string{"attr", "attr-*", "open"}),
	kindChoice:   keySet(occurrenceKeys, []string{"score-max"}, findingKeys),
	kindSequence: keySet([]string{"score-max"}, findingKeys),
	kindComp:     keySet(occurrenceKeys, []string{"score-max"}, findingKeys, []string{"open"}),
	kindLD:       keySet(occurrenceKeys, []string{"score", "score-max"}, findingKeys),
	kindProp:     keySet([]string{"optional", "every"}, textKeys, []string{"value", "score", "severity", "penalty"}),
}

// structural attributes of val:* elements, they are not rules
var structuralAttrs = map[string][]string{
	"val:selector":  {"for"},
	"val:wildcard":  {"for"},
	"val:include":   {"path"},
	"val:comp":      {"path"},
	"val:component": {"marker", "selector"},
	"val:ld":        {"type"},
	"val:prop":      {"path"},
	"val:settings":  {"exclude", "marker-attribute", "penalty-error", "penalty-warn", "penalty-info"},
}

// rule names that are easily written without val: on html elements
var forgottenPrefix = keySet(textKeys, scoreKeys, []string{"optional", "forbidden", "severity", "penalty", "penalty-max", "val"})

func defaultOccurrence(k kind, inChoice bool) occurrence {
	switch {
	case k == kindSelector:
		return occurrence{1, unbounded}
	case k == kindChoice, k == kindSequence:
		return occurrence{0, unbounded}
	case k == kindComp && inChoice:
		return occurrence{0, unbounded}
	}
	return occurrence{1, 1}
}

func newElement(k kind, n *rawNode, skip *wildcard) *element {
	return &element{
		kind:       k,
		occ:        defaultOccurrence(k, false),
		text:       newTextRules(),
		scoreMax:   unbounded,
		severity:   SeverityError,
		penalty:    -1,
		penaltyMax: unbounded,
		skip:       skip,
		source:     n.source(),
		value:      valueRules{penalty: -1},
	}
}

// ruleAttrs splits the attributes of n into rules and literals and checks the spelling
func (c *compiler) ruleAttrs(n *rawNode) (rules []html.Attribute, literals []html.Attribute) {
	seen := map[string]bool{}
	isVal := strings.HasPrefix(n.name, "val:")
	structural := map[string]bool{}
	for _, s := range structuralAttrs[n.name] {
		structural[s] = true
	}
	for _, a := range n.attrs {
		if seen[a.Key] {
			c.errorf(n, "duplicate attribute %q", a.Key)
			continue
		}
		seen[a.Key] = true
		switch {
		case isVal && structural[a.Key]:
		case isVal && strings.HasPrefix(a.Key, "val:"):
			c.errorf(n, "<%s> takes its rules without prefix: %q instead of %q", n.name, strings.TrimPrefix(a.Key, "val:"), a.Key)
		case isVal:
			rules = append(rules, a)
		case strings.HasPrefix(a.Key, "val:"):
			rules = append(rules, html.Attribute{Key: strings.TrimPrefix(a.Key, "val:"), Val: a.Val})
		default:
			if forgottenPrefix[a.Key] {
				c.errorf(n, "attribute %q looks like a rule, did you mean %q?", a.Key, "val:"+a.Key)
				continue
			}
			literals = append(literals, a)
		}
	}
	return rules, literals
}

// applyRules parses rule attributes into e
func (c *compiler) applyRules(n *rawNode, e *element, rules []html.Attribute, inChoice bool) {
	allowed := allowedRules[e.kind]
	e.occ = defaultOccurrence(e.kind, inChoice)
	e.rules = attrText(rules)
	var (
		minSet, maxSet, optional bool
		minVal, maxVal           int
	)
	number := func(a html.Attribute) (int, bool) {
		v, err := strconv.Atoi(a.Val)
		if err != nil || v < 0 {
			c.errorf(n, "%s needs a number, got %q", a.Key, a.Val)
			return 0, false
		}
		return v, true
	}
	for _, a := range rules {
		key := a.Key
		if strings.HasPrefix(key, "attr-") {
			key = "attr-*"
		}
		if !allowed[key] {
			c.errorf(n, "unknown rule %q on <%s>", a.Key, n.name)
			continue
		}
		switch key {
		case "optional":
			optional = true
		case "forbidden":
			minSet, maxSet, minVal, maxVal = true, true, 0, 0
		case "count":
			if v, ok := number(a); ok {
				minSet, maxSet, minVal, maxVal = true, true, v, v
			}
		case "min":
			if v, ok := number(a); ok {
				minSet, minVal = true, v
			}
		case "max":
			if a.Val == "*" {
				maxSet, maxVal = true, unbounded
			} else if v, ok := number(a); ok {
				maxSet, maxVal = true, v
			}
		case "min-length", "max-length", "min-words", "max-words":
			v, ok := number(a)
			if !ok {
				continue
			}
			switch key {
			case "min-length":
				e.text.minLength = v
			case "max-length":
				e.text.maxLength = v
			case "min-words":
				e.text.minWords = v
			case "max-words":
				e.text.maxWords = v
			}
		case "attr-*", "attr":
			attrName := strings.TrimPrefix(a.Key, "attr-")
			spec := a.Val
			if key == "attr" {
				// v1 form: "name; rule; rule"
				attrName, spec, _ = strings.Cut(a.Val, ";")
				attrName = strings.TrimSpace(attrName)
			}
			vr, err := parseValueRules(spec)
			if err != nil {
				c.errorf(n, "%s: %s", a.Key, err)
				continue
			}
			e.attrRules = append(e.attrRules, &attrRule{attr: attrName, valueRules: vr})
		case "value":
			vr, err := parseValueRules(a.Val)
			if err != nil {
				c.errorf(n, "value: %s", err)
				continue
			}
			e.value = vr
		case "score":
			if v, ok := number(a); ok {
				e.score = v
			}
		case "score-max":
			if v, ok := number(a); ok {
				e.scoreMax = v
			}
		case "score-per":
			unit, k, _ := strings.Cut(a.Val, ":")
			v, err := strconv.Atoi(k)
			if (unit != "words" && unit != "chars") || err != nil || v <= 0 {
				c.errorf(n, `score-per needs "words:n" or "chars:n", got %q`, a.Val)
				continue
			}
			e.scorePer, e.scoreUnit = unit, v
		case "severity":
			sev, err := parseSeverity(a.Val)
			if err != nil {
				c.errorf(n, "%s", err)
				continue
			}
			e.severity, e.severitySet = sev, true
		case "penalty":
			if v, ok := number(a); ok {
				e.penalty = v
			}
		case "penalty-max":
			if v, ok := number(a); ok {
				e.penaltyMax = v
			}
		case "open":
			e.open = true
		case "every":
			e.every = true
		}
	}
	if optional {
		if minSet {
			c.errorf(n, "optional together with min or count")
		}
		e.occ.min = 0
	}
	if minSet {
		e.occ.min = minVal
		if !maxSet {
			e.occ.max = unbounded
		}
	}
	if maxSet {
		e.occ.max = maxVal
	}
	if e.occ.max != unbounded && e.occ.min > e.occ.max {
		c.errorf(n, "min %d is greater than max %d", e.occ.min, e.occ.max)
	}
}

func (c *compiler) compileNodes(nodes []*rawNode, parent *rawNode, skip *wildcard, inChoice bool) (elements []*element) {
	for _, n := range nodes {
		elements = append(elements, c.compileNode(n, parent, skip, inChoice)...)
	}
	return elements
}

func (c *compiler) enter(key string, n *rawNode) bool {
	for _, s := range c.stack {
		if s == key {
			c.errorf(n, "cycle: %s -> %s", strings.Join(c.stack, " -> "), key)
			return false
		}
	}
	c.stack = append(c.stack, key)
	return true
}

func (c *compiler) leave() {
	c.stack = c.stack[:len(c.stack)-1]
}

// target resolves the path attribute of an include or comp
func (c *compiler) target(n *rawNode) (key string, file *rawNode) {
	p, ok := n.attr("path")
	if !ok || p == "" {
		c.errorf(n, "<%s> needs a path", n.name)
		return "", nil
	}
	key = resolvePath(fileKey(n.file), p)
	file = c.root.files[key]
	if file == nil {
		c.errorf(n, "%s: %s not found", n.name, key)
		return "", nil
	}
	return key, file
}

func (c *compiler) compileNode(n *rawNode, parent *rawNode, skip *wildcard, inChoice bool) []*element {
	switch n.name {
	case "val:include":
		c.onlyStructural(n)
		c.noChildren(n)
		key, file := c.target(n)
		if file == nil || !c.enter(key, n) {
			return nil
		}
		defer c.leave()
		return c.compileNodes(file.children, parent, skip, inChoice)
	case "val:wildcard":
		c.onlyStructural(n)
		f, ok := n.attr("for")
		if !ok {
			c.errorf(n, `<val:wildcard> needs a for="css" attribute`)
		}
		var scope *wildcard
		if strings.TrimSpace(f) != "" {
			sel, err := cascadia.Compile(f)
			if err != nil {
				c.errorf(n, "invalid for %q: %s", f, err)
			}
			scope = &wildcard{match: sel, text: strings.TrimSpace(f)}
		}
		return c.compileNodes(n.children, parent, scope, inChoice)
	case "val:comp":
		return c.compileComp(n, parent, skip, inChoice)
	case "val:settings":
		if parent == nil || parent.name != "html" {
			c.errorf(n, "<val:settings> has to be a direct child of <html>")
			return nil
		}
		c.compileSettings(n)
		return nil
	case "val:component":
		c.errorf(n, "<val:component> is only allowed as the root of a component file")
		return nil
	case "val:selector":
		e := newElement(kindSelector, n, skip)
		f, _ := n.attr("for")
		sel, err := cascadia.Compile(f)
		if err != nil || f == "" {
			c.errorf(n, "<val:selector> needs a valid for=\"css\", got %q", f)
		}
		e.selector = sel
		e.label = "val:selector(" + f + ")"
		c.finishElement(n, e, inChoice)
		return []*element{e}
	case "val:choice", "val:sequence":
		k := kindChoice
		if n.name == "val:sequence" {
			k = kindSequence
		}
		e := newElement(k, n, skip)
		e.label = n.name
		rules, _ := c.ruleAttrs(n)
		c.applyRules(n, e, rules, false)
		for _, child := range c.compileNodes(n.children, n, skip, k == kindChoice) {
			if child.kind != kindComp || child.comp == nil {
				c.errorf(n, "<%s> may only contain val:comp of components (%s)", n.name, child.source)
				continue
			}
			e.children = append(e.children, child)
		}
		return []*element{e}
	case "val:ld":
		e := newElement(kindLD, n, skip)
		e.ldType, _ = n.attr("type")
		if e.ldType == "" {
			c.errorf(n, "<val:ld> needs a type")
		}
		e.label = "val:ld(" + e.ldType + ")"
		rules, _ := c.ruleAttrs(n)
		c.applyRules(n, e, rules, false)
		for _, child := range n.children {
			if child.name != "val:prop" {
				c.errorf(child, "<val:ld> may only contain <val:prop>")
				continue
			}
			p := newElement(kindProp, child, nil)
			path, _ := child.attr("path")
			if path == "" {
				c.errorf(child, "<val:prop> needs a path")
			}
			p.propPath = strings.Split(path, ".")
			p.label = path
			propRules, _ := c.ruleAttrs(child)
			c.applyRules(child, p, propRules, false)
			c.noChildren(child)
			e.children = append(e.children, p)
		}
		return []*element{e}
	}
	if strings.HasPrefix(n.name, "val:") {
		c.errorf(n, "unknown element <%s>", n.name)
		return nil
	}
	e := newElement(kindElement, n, skip)
	e.name = n.name
	c.finishElement(n, e, inChoice)
	return []*element{e}
}

// finishElement parses rules and literals of an html element or val:selector and compiles its children
func (c *compiler) finishElement(n *rawNode, e *element, inChoice bool) {
	rules, literals := c.ruleAttrs(n)
	c.applyRules(n, e, rules, inChoice)
	for _, l := range literals {
		e.literals = append(e.literals, literal{key: l.Key, val: l.Val})
	}
	if len(literals) > 0 {
		e.rules = strings.TrimSpace(attrText(literals) + " " + e.rules)
	}
	if e.kind == kindElement {
		e.label = e.name
		if len(e.literals) > 0 {
			e.label += "[" + e.literals[0].key + "=" + e.literals[0].val + "]"
		}
	}
	// the wildcard scope is lexical: children inherit it
	e.children = c.compileNodes(n.children, n, e.skip, false)
	c.checkContext(n, e.children)
}

// checkContext reports components that are used more than once in one component context
func (c *compiler) checkContext(n *rawNode, children []*element) {
	used := map[*component]bool{}
	for _, child := range children {
		uses := []*element{child}
		if child.kind == kindChoice || child.kind == kindSequence {
			uses = child.children
		}
		for _, use := range uses {
			if use.kind != kindComp || use.comp == nil {
				continue
			}
			if used[use.comp] {
				c.errorf(n, "%s is used more than once in this context", use.comp.path)
			}
			used[use.comp] = true
		}
	}
}

// onlyStructural reports every attribute of n that is not structural
func (c *compiler) onlyStructural(n *rawNode) {
	rules, _ := c.ruleAttrs(n)
	for _, a := range rules {
		c.errorf(n, "unknown attribute %q on <%s>", a.Key, n.name)
	}
}

func (c *compiler) noChildren(n *rawNode) {
	if len(n.children) > 0 {
		c.errorf(n, "<%s> must not have children, close it with />", n.name)
	}
}

func (c *compiler) compileSettings(n *rawNode) {
	if c.settingsNode != nil {
		c.errorf(n, "duplicate <val:settings>, first one at %s", c.settingsNode.source())
		return
	}
	c.settingsNode = n
	c.onlyStructural(n)
	for _, a := range n.attrs {
		switch a.Key {
		case "exclude":
			sel, err := cascadia.Compile(a.Val)
			if err != nil {
				c.errorf(n, "invalid exclude %q: %s", a.Val, err)
				continue
			}
			c.settings.exclude = sel
		case "marker-attribute":
			c.settings.markerAttribute = a.Val
		case "penalty-error", "penalty-warn", "penalty-info":
			v, err := strconv.Atoi(a.Val)
			if err != nil || v < 0 {
				c.errorf(n, "%s needs a number, got %q", a.Key, a.Val)
				continue
			}
			c.settings.penalties[Severity(strings.TrimPrefix(a.Key, "penalty-"))] = v
		}
	}
	c.noChildren(n)
}

// compileComp instantiates the single root element of a file, the attributes of
// val:comp override or extend the ones of the root
func (c *compiler) compileComp(n *rawNode, parent *rawNode, skip *wildcard, inChoice bool) []*element {
	c.noChildren(n)
	key, file := c.target(n)
	if file == nil {
		return nil
	}
	if len(file.children) != 1 {
		c.errorf(n, "%s can not be instantiated, it has %d top level elements instead of one - use val:include", key, len(file.children))
		return nil
	}
	if !c.enter(key, n) {
		return nil
	}
	defer c.leave()
	root := file.children[0]
	merged := c.mergeAttrs(root, n)
	if root.name != "val:component" {
		return c.compileNode(merged, parent, skip, inChoice)
	}
	comp := c.root.components[key]
	c.compileComponent(comp)
	e := newElement(kindComp, n, skip)
	e.comp = comp
	e.label = comp.label
	rules, _ := c.ruleAttrs(merged)
	c.applyRules(n, e, rules, inChoice)
	return []*element{e}
}

func (c *compiler) mergeAttrs(root *rawNode, site *rawNode) *rawNode {
	isVal := strings.HasPrefix(root.name, "val:")
	canonical := func(k string) string {
		if isVal {
			return "val:" + k
		}
		return k
	}
	occurrence := keySet(occurrenceKeys)
	siteHasOccurrence := false
	var overrides []html.Attribute
	for _, a := range site.attrs {
		if a.Key == "path" {
			continue
		}
		if isVal && !strings.HasPrefix(a.Key, "val:") {
			c.errorf(site, "%s: literal attribute %q can not be set on <%s>", site.name, a.Key, root.name)
			continue
		}
		name := strings.TrimPrefix(a.Key, "val:")
		if root.name == "val:component" && (name == "marker" || name == "selector") {
			c.errorf(site, "%s of a component can not be overridden", name)
			continue
		}
		if strings.HasPrefix(a.Key, "val:") && occurrence[name] {
			siteHasOccurrence = true
		}
		overrides = append(overrides, a)
	}
	merged := &rawNode{name: root.name, children: root.children, file: root.file, line: root.line}
	for _, a := range root.attrs {
		key := a.Key
		if isVal {
			for _, s := range structuralAttrs[root.name] {
				if s == key {
					key = "#" + key // structural attributes are kept as they are
				}
			}
		}
		if !strings.HasPrefix(key, "#") {
			key = canonical(key)
		}
		if siteHasOccurrence && strings.HasPrefix(key, "val:") && occurrence[strings.TrimPrefix(key, "val:")] {
			continue
		}
		overridden := false
		for _, o := range overrides {
			if o.Key == key {
				overridden = true
			}
		}
		if !overridden {
			merged.attrs = append(merged.attrs, a)
		}
	}
	for _, o := range overrides {
		key := o.Key
		if isVal {
			key = strings.TrimPrefix(key, "val:")
		}
		merged.attrs = append(merged.attrs, html.Attribute{Key: key, Val: o.Val})
	}
	return merged
}

func (c *compiler) compileComponent(comp *component) {
	if comp.compiled {
		return
	}
	comp.compiled = true
	n := comp.node
	rules, _ := c.ruleAttrs(n)
	c.applyRules(n, newElement(kindComp, n, nil), rules, false)
	comp.marker, _ = n.attr("marker")
	if s, ok := n.attr("selector"); ok && s != "" {
		sel, err := cascadia.Compile(s)
		if err != nil {
			c.errorf(n, "invalid selector %q: %s", s, err)
		}
		comp.selector = sel
	}
	if comp.marker == "" && comp.selector == nil {
		c.errorf(n, "a component needs a marker, a selector or both")
	}
	_, comp.open = n.attr("open")
	// wildcards do not reach into a component
	comp.body = c.compileNodes(n.children, n, nil, false)
	c.checkContext(n, comp.body)
}

// fileKey turns a display file name into a root relative key without extension
func fileKey(file string) string {
	return "/" + strings.TrimSuffix(file, FileSuffix)
}

// attrText writes attributes as in a schema: key=value, or key for booleans
func attrText(attrs []html.Attribute) string {
	parts := make([]string, len(attrs))
	for i, a := range attrs {
		switch {
		case a.Val == "":
			parts[i] = a.Key
		case strings.ContainsAny(a.Val, " \"'"):
			parts[i] = a.Key + "=" + strconv.Quote(a.Val)
		default:
			parts[i] = a.Key + "=" + a.Val
		}
	}
	return strings.Join(parts, " ")
}
