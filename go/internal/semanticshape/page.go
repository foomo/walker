package semanticshape

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
)

// instance is a recognized component in a page - opaque for the shape around it
type instance struct {
	node      *html.Node
	comp      *component   // nil for ambiguous selectors
	ambiguous []*component // more than one selector matched
}

type ldObject struct {
	script *html.Node
	data   map[string]any
}

// page is a parsed page, prepared for validation. Excluded nodes stay in the
// tree, so CSS selectors and locators work on the page as delivered.
type page struct {
	doc       *html.Node
	excluded  map[*html.Node]bool
	instances map[*html.Node]*instance
	ambiguous []*instance
	ld        []ldObject
	order     map[*html.Node]int // document order
}

var ldScript = cascadia.MustCompile(`script[type="application/ld+json"]`)

func newPage(htmlBytes []byte, r *Root, set settings) (*page, error) {
	doc, err := html.Parse(bytes.NewReader(htmlBytes))
	if err != nil {
		return nil, err
	}
	p := &page{
		doc:       doc,
		excluded:  map[*html.Node]bool{},
		instances: map[*html.Node]*instance{},
		order:     map[*html.Node]int{},
	}
	p.prepare(doc, false, nil, r, set)
	return p, nil
}

func (p *page) prepare(n *html.Node, excluded bool, enclosing []*component, r *Root, set settings) {
	p.order[n] = len(p.order)
	if n.Type == html.ElementNode {
		if ldScript.Match(n) {
			p.readLD(n)
		}
		if !excluded {
			_, hidden := attr(n, "hidden")
			switch {
			case hidden, n.Data == "script", n.Data == "style", n.Data == "template", n.Data == "noscript":
				excluded = true
			case set.exclude != nil && set.exclude.Match(n):
				excluded = true
			}
		}
		if excluded {
			p.excluded[n] = true
		} else if inst := p.recognize(n, enclosing, r, set); inst != nil {
			p.instances[n] = inst
			if inst.ambiguous != nil {
				p.ambiguous = append(p.ambiguous, inst)
			}
			if inst.comp != nil {
				enclosing = append(enclosing, inst.comp)
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		p.prepare(c, excluded, enclosing, r, set)
	}
}

func (p *page) recognize(n *html.Node, enclosing []*component, r *Root, set settings) *instance {
	// a marker without a component shape is ignored, as if it was not there
	if marker, _ := attr(n, set.markerAttribute); marker != "" {
		if comp, ok := r.markers[marker]; ok {
			return &instance{node: n, comp: comp}
		}
	}
	var matches []*component
CompLoop:
	for _, comp := range r.componentList {
		if comp.selector == nil || !comp.selector.Match(n) {
			continue
		}
		// a selector match inside an instance of the same component is part of it
		for _, e := range enclosing {
			if e == comp {
				continue CompLoop
			}
		}
		matches = append(matches, comp)
	}
	switch len(matches) {
	case 0:
		return nil
	case 1:
		return &instance{node: n, comp: matches[0]}
	}
	return &instance{node: n, ambiguous: matches}
}

func (p *page) readLD(script *html.Node) {
	var data any
	if err := json.Unmarshal([]byte(textOf(script, nil)), &data); err != nil {
		return
	}
	var collect func(v any)
	collect = func(v any) {
		switch t := v.(type) {
		case []any:
			for _, item := range t {
				collect(item)
			}
		case map[string]any:
			if graph, ok := t["@graph"]; ok {
				collect(graph)
			}
			if _, ok := t["@type"]; ok {
				p.ld = append(p.ld, ldObject{script: script, data: t})
			}
		}
	}
	collect(data)
}

func (o ldObject) hasType(t string) bool {
	switch v := o.data["@type"].(type) {
	case string:
		return v == t
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == t {
				return true
			}
		}
	}
	return false
}

// text is the normalized text content of n without excluded subtrees
func (p *page) text(n *html.Node) string {
	return normalizeText(textOf(n, p.excluded))
}

func textOf(n *html.Node, excluded map[*html.Node]bool) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if excluded[n] {
			return
		}
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
			sb.WriteString(" ")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c)
	}
	return sb.String()
}

// locator is a CSS path to n, usable with document.querySelector
func locator(n *html.Node) string {
	var parts []string
	for ; n != nil && n.Type == html.ElementNode; n = n.Parent {
		part := n.Data
		index, count := 0, 0
		for s := n.Parent.FirstChild; s != nil; s = s.NextSibling {
			if s.Type == html.ElementNode && s.Data == n.Data {
				count++
				if s == n {
					index = count
				}
			}
		}
		if count > 1 {
			part += ":nth-of-type(" + strconv.Itoa(index) + ")"
		}
		parts = append([]string{part}, parts...)
	}
	return strings.Join(parts, " > ")
}

func attr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

// isInside reports whether n is root or one of its descendants
func isInside(n, root *html.Node) bool {
	for ; n != nil; n = n.Parent {
		if n == root {
			return true
		}
	}
	return false
}
