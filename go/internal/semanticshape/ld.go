package semanticshape

import (
	"encoding/json"
	"fmt"
	"strconv"

	"golang.org/x/net/html"
)

// validateLD validates the JSON-LD objects of a type, page wide or inside the
// current component instance
func (v *validator) validateLD(ctx *html.Node, e *element, path string, sc scope) (int, *ScoreNode) {
	var (
		objects []ldObject
		scripts []*html.Node
	)
	for _, o := range v.page.ld {
		if o.hasType(e.ldType) && (sc.ldRoot == nil || isInside(o.script, sc.ldRoot)) {
			objects = append(objects, o)
			scripts = append(scripts, o.script)
		}
	}
	tn := v.enterElement(e, path)
	defer v.leave(tn)
	counted := v.checkOccurrence(ctx, e, scripts, path, sc, "ld-occurrence")
	tn.match(scripts...)
	tn.context(ctx)
	score := &ScoreNode{Path: joinPath(path, e.label), Max: e.max}
	total := 0
	for i := range counted {
		o := objects[i]
		objectPath := joinPath(path, indexed(e.label, i, len(objects)))
		mt := tn
		if len(objects) > 1 {
			mt = v.enter("match", indexed(e.label, i, len(objects)), objectPath, e.source, "", e.nodeMax)
			mt.match(o.script)
		}
		ok := true
		points := 0
		for _, prop := range e.children {
			propPath := objectPath + "/" + prop.label
			values := ldValues(o.data, prop.propPath)
			pt := v.enter("prop", prop.label, propPath, prop.source, prop.rules, prop.score)
			if len(values) == 0 {
				if prop.occ.min > 0 {
					v.add(sc, prop.severity, v.penaltyFor(prop.severity, prop.penalty), "ld-prop:required", propPath, o.script, prop.source, "missing")
					ok = false
				}
				v.leave(pt)
				continue
			}
			pt.match(o.script)
			if prop.every {
				if v.checkEvery(sc, prop, o, propPath) {
					points += prop.score
					pt.points(prop.score)
				} else if prop.occ.min > 0 {
					ok = false
				}
				v.leave(pt)
				continue
			}
			// any value of an array may satisfy the rules, report the first one otherwise
			var first []propFailure
			passed := false
			for _, val := range values {
				failures := propFailures(prop, val)
				if len(failures) == 0 {
					passed = true
					break
				}
				if first == nil {
					first = failures
				}
			}
			if passed {
				points += prop.score
				pt.points(prop.score)
				v.leave(pt)
				continue
			}
			for _, f := range first {
				v.add(sc, f.severity, v.penaltyFor(f.severity, f.penalty), "ld-prop:"+f.rule, propPath, o.script, prop.source, f.message)
			}
			if prop.occ.min > 0 {
				ok = false
			}
			v.leave(pt)
		}
		if ok {
			points += e.score
		}
		total += points
		if mt != tn {
			mt.points(points)
			v.leave(mt)
		}
	}
	v.reportSurplus(e, scripts, path, sc, "ld-occurrence")
	if e.scoreMax != unbounded && total > e.scoreMax {
		total = e.scoreMax
	}
	score.Points = total
	tn.points(total)
	return total, score
}

// checkEvery checks every value of a property path - every array element has
// to pass, and has to have the property if it is required. Each failing
// element is a finding of its own, indexed in the order of the document.
func (v *validator) checkEvery(sc scope, prop *element, o ldObject, propPath string) bool {
	values := ldEach(o.data, prop.propPath)
	passed := true
	for i, val := range values {
		elementPath := propPath
		if len(values) > 1 {
			elementPath = fmt.Sprintf("%s[%d]", propPath, i+1)
		}
		if !val.found {
			if prop.occ.min > 0 {
				v.add(sc, prop.severity, v.penaltyFor(prop.severity, prop.penalty), "ld-prop:required", elementPath, o.script, prop.source, "missing")
				passed = false
			}
			continue
		}
		for _, f := range propFailures(prop, val.value) {
			v.add(sc, f.severity, v.penaltyFor(f.severity, f.penalty), "ld-prop:"+f.rule, elementPath, o.script, prop.source, f.message)
			passed = false
		}
	}
	return passed
}

// ldValue is a value of a property path, or the place where it is missing
type ldValue struct {
	value string
	found bool
}

// ldEach follows a dot path like ldValues, but keeps a missing value for every
// array element that does not have the property
func ldEach(v any, steps []string) (values []ldValue) {
	switch t := v.(type) {
	case []any:
		for _, item := range t {
			values = append(values, ldEach(item, steps)...)
		}
		return values
	case map[string]any:
		if len(steps) == 0 {
			return toLDValues(ldValues(t, nil))
		}
		next, ok := t[steps[0]]
		if !ok || next == nil {
			return []ldValue{{}}
		}
		return ldEach(next, steps[1:])
	case nil:
		return []ldValue{{}}
	}
	if len(steps) > 0 {
		return []ldValue{{}}
	}
	return toLDValues(ldValues(v, nil))
}

func toLDValues(strings []string) (values []ldValue) {
	for _, s := range strings {
		values = append(values, ldValue{value: s, found: true})
	}
	return values
}

type propFailure struct {
	rule     string
	message  string
	severity Severity
	penalty  int
}

func propFailures(prop *element, val string) (failures []propFailure) {
	for _, f := range prop.text.check(normalizeText(val)) {
		failures = append(failures, propFailure{f.rule, f.message, prop.severity, prop.penalty})
	}
	failed, messages := prop.value.failures(val)
	sev, penalty := prop.severity, prop.penalty
	if prop.value.severity != "" {
		sev = prop.value.severity
	}
	if prop.value.penalty >= 0 {
		penalty = prop.value.penalty
	}
	for i, check := range failed {
		failures = append(failures, propFailure{check.name, messages[i], sev, penalty})
	}
	return failures
}

// ldValues follows a dot path into a JSON-LD value, arrays fan out
func ldValues(v any, steps []string) (values []string) {
	switch t := v.(type) {
	case nil:
		return nil
	case []any:
		for _, item := range t {
			values = append(values, ldValues(item, steps)...)
		}
		return values
	case map[string]any:
		if len(steps) == 0 {
			data, _ := json.Marshal(t)
			return []string{string(data)}
		}
		return ldValues(t[steps[0]], steps[1:])
	}
	if len(steps) > 0 {
		return nil
	}
	switch t := v.(type) {
	case string:
		return []string{t}
	case float64:
		return []string{strconv.FormatFloat(t, 'f', -1, 64)}
	case bool:
		return []string{strconv.FormatBool(t)}
	}
	return nil
}
