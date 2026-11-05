package semanticshape

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// PageData is the invisible part of a page - the html element, the head and
// the JSON-LD documents - with the same locators as a trace
type PageData struct {
	HTML       *HeadElement
	Head       []*HeadElement
	LinkedData []*LDBlock
}

// HeadElement is an element of the head, or the html element itself
type HeadElement struct {
	Tag        string
	Attributes []HeadAttribute
	// Text is the text of a title, or a note on inline scripts and styles
	Text    string
	Locator string
}

type HeadAttribute struct {
	Key   string
	Value string
}

// LDBlock is a script[type="application/ld+json"]
type LDBlock struct {
	Locator string
	// Types are the @type values of the top level objects and of @graph
	Types []string
	// JSON is indented, or as found in the page if it does not parse
	JSON  string
	Error string
}

// ExtractPageData parses a page like the validator does and returns its invisible parts
func ExtractPageData(htmlBytes []byte) (*PageData, error) {
	doc, err := html.Parse(bytes.NewReader(htmlBytes))
	if err != nil {
		return nil, err
	}
	data := &PageData{}
	root := firstElement(doc, "html")
	if root == nil {
		return data, nil
	}
	data.HTML = headElement(root, "")
	if head := firstElement(root, "head"); head != nil {
		for c := head.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode {
				data.Head = append(data.Head, headElement(c, headText(c)))
			}
		}
	}
	for _, script := range ldScript.MatchAll(doc) {
		data.LinkedData = append(data.LinkedData, ldBlock(script))
	}
	return data, nil
}

func firstElement(parent *html.Node, tag string) *html.Node {
	for c := parent.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			return c
		}
	}
	return nil
}

func headElement(n *html.Node, text string) *HeadElement {
	e := &HeadElement{Tag: n.Data, Text: text, Locator: locator(n)}
	for _, a := range n.Attr {
		e.Attributes = append(e.Attributes, HeadAttribute{Key: a.Key, Value: a.Val})
	}
	return e
}

// headText is the text of a title, and a note for inline content
func headText(n *html.Node) string {
	text := textOf(n, nil)
	switch n.Data {
	case "title":
		return normalizeText(text)
	case "script":
		if ldScript.Match(n) {
			return "JSON-LD, see below"
		}
		if _, ok := attr(n, "src"); ok || strings.TrimSpace(text) == "" {
			return ""
		}
		return fmt.Sprintf("%d bytes inline", len(text))
	case "style":
		return fmt.Sprintf("%d bytes inline", len(text))
	case "noscript":
		return normalizeText(text)
	}
	return ""
}

func ldBlock(script *html.Node) *LDBlock {
	raw := strings.TrimSpace(textOf(script, nil))
	block := &LDBlock{Locator: locator(script), JSON: raw}
	var data any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		block.Error = err.Error()
		return block
	}
	indented := &bytes.Buffer{}
	if json.Indent(indented, []byte(raw), "", "  ") == nil {
		block.JSON = indented.String()
	}
	var collect func(v any)
	collect = func(v any) {
		switch t := v.(type) {
		case []any:
			for _, item := range t {
				collect(item)
			}
		case map[string]any:
			switch types := t["@type"].(type) {
			case string:
				block.Types = append(block.Types, types)
			case []any:
				for _, item := range types {
					if s, ok := item.(string); ok {
						block.Types = append(block.Types, s)
					}
				}
			}
			if graph, ok := t["@graph"]; ok {
				collect(graph)
			}
		}
	}
	collect(data)
	return block
}
