package htmlschema

import (
	"golang.org/x/net/html"
)

// reachable visits the element children of ctx and, through nodes matching
// skip, their descendants. Excluded nodes are ignored. Component instances are
// visited, but never searched through.
func (p *page) reachable(ctx *html.Node, skip *wildcard, visit func(n *html.Node)) {
	for c := ctx.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || p.excluded[c] {
			continue
		}
		visit(c)
		if p.instances[c] != nil {
			continue
		}
		if skip != nil && skip.match.Match(c) {
			p.reachable(c, skip, visit)
		}
	}
}

// candidates are the page nodes e matches in ctx, in document order
func (p *page) candidates(ctx *html.Node, e *element) (nodes []*html.Node) {
	switch e.kind {
	case kindSelector:
		for _, n := range e.selector.MatchAll(ctx) {
			if n != ctx && !p.isExcluded(n) {
				nodes = append(nodes, n)
			}
		}
	case kindComp:
		p.reachable(ctx, e.skip, func(n *html.Node) {
			if inst := p.instances[n]; inst != nil && inst.comp == e.comp {
				nodes = append(nodes, n)
			}
		})
	default:
		p.reachable(ctx, e.skip, func(n *html.Node) {
			if p.instances[n] == nil && matchesElement(n, e) {
				nodes = append(nodes, n)
			}
		})
	}
	return nodes
}

// instancesIn are all instances reachable in ctx
func (p *page) instancesIn(ctx *html.Node, skip *wildcard) (instances []*instance) {
	p.reachable(ctx, skip, func(n *html.Node) {
		if inst := p.instances[n]; inst != nil {
			instances = append(instances, inst)
		}
	})
	return instances
}

func (p *page) isExcluded(n *html.Node) bool {
	for ; n != nil; n = n.Parent {
		if p.excluded[n] {
			return true
		}
	}
	return false
}

func matchesElement(n *html.Node, e *element) bool {
	if n.Data != e.name {
		return false
	}
	for _, l := range e.literals {
		val, ok := attr(n, l.key)
		if l.val == "*" {
			if !ok || val == "" {
				return false
			}
		} else if val != l.val {
			return false
		}
	}
	return true
}
