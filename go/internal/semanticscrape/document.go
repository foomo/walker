package semanticscrape

import (
	"bytes"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

type (
	ElementType     string
	DocumentElement struct {
		ElementType ElementType
		Selector    string
		// Text is the accessible name of a heading
		Text string
		// Label is the accessible name the author gave a container with
		// aria-labelledby or aria-label, containers never take it from content
		Label string
		// Links of a nav, in document order - what a nav is about. With
		// content: the links in a text and the link of a heading or image.
		Links []Link
		// Src of an image
		Src      string
		Children []DocumentElement

		// node is the element in the page, while the extraction runs
		node *html.Node
	}

	// Options of ExtractDocument
	Options struct {
		// Content adds the content of the containers to the outline, between
		// their elements in reading order: texts with their links, images,
		// figures, lists and tables
		Content bool
	}

	Link struct {
		// Text is the accessible name of the link
		Text     string
		Href     string
		Selector string
	}

	// Document is the gist of a page: its head in three values and the outline
	// of its body
	Document struct {
		// Lang of the html element
		Lang        string
		Title       string
		Description string
		// Body is the outline of the body - the landmarks main, header, nav and
		// footer are elements of it, next to the articles, sections, asides and
		// headings
		Body []DocumentElement
	}
)

const (
	ElementTypeH1 ElementType = "h1"
	ElementTypeH2 ElementType = "h2"
	ElementTypeH3 ElementType = "h3"
	ElementTypeH4 ElementType = "h4"
	ElementTypeH5 ElementType = "h5"
	ElementTypeH6 ElementType = "h6"

	ElementTypeArticle ElementType = "article"
	ElementTypeSection ElementType = "section"
	ElementTypeAside   ElementType = "aside"
	ElementTypeDiv     ElementType = "div"

	ElementTypeMain   ElementType = "main"
	ElementTypeHeader ElementType = "header"
	ElementTypeFooter ElementType = "footer"
	ElementTypeNav    ElementType = "nav"

	// content, see Options.Content
	ElementTypeText   ElementType = "text"
	ElementTypeImage  ElementType = "img"
	ElementTypeFigure ElementType = "figure"
	// lists: ul and ol with li items, dl with dt and dd items
	ElementTypeUnorderedList ElementType = "ul"
	ElementTypeOrderedList   ElementType = "ol"
	ElementTypeListItem      ElementType = "li"
	ElementTypeDefinitions   ElementType = "dl"
	ElementTypeTerm          ElementType = "dt"
	ElementTypeDefinition    ElementType = "dd"
	// tables: table with tr rows of th and td cells
	ElementTypeTable      ElementType = "table"
	ElementTypeRow        ElementType = "tr"
	ElementTypeHeaderCell ElementType = "th"
	ElementTypeCell       ElementType = "td"
)

var elementTypes = map[string]ElementType{
	string(ElementTypeH1):      ElementTypeH1,
	string(ElementTypeH2):      ElementTypeH2,
	string(ElementTypeH3):      ElementTypeH3,
	string(ElementTypeH4):      ElementTypeH4,
	string(ElementTypeH5):      ElementTypeH5,
	string(ElementTypeH6):      ElementTypeH6,
	string(ElementTypeArticle): ElementTypeArticle,
	string(ElementTypeSection): ElementTypeSection,
	string(ElementTypeAside):   ElementTypeAside,
	string(ElementTypeHeader):  ElementTypeHeader,
	string(ElementTypeFooter):  ElementTypeFooter,
	string(ElementTypeNav):     ElementTypeNav,
}

// ARIA roles with the element type they stand for, role="heading" takes its
// level from aria-level
var roleTypes = map[string]ElementType{
	"article":       ElementTypeArticle,
	"region":        ElementTypeSection,
	"complementary": ElementTypeAside,
	"banner":        ElementTypeHeader,
	"contentinfo":   ElementTypeFooter,
	"navigation":    ElementTypeNav,
}

// a header or footer inside these is the header or footer of that element, not
// of the page - the page's header and footer are landmarks, theirs are not
var landmarkScopes = map[string]bool{
	"article": true, "aside": true, "main": true, "nav": true, "section": true,
	"role:article": true, "role:complementary": true, "role:main": true, "role:navigation": true, "role:region": true,
}

// skipped subtrees, they never render content - the parser runs with scripting
// enabled, so <noscript> holds its markup as raw text
var skipTags = map[string]bool{"script": true, "style": true, "template": true, "noscript": true}

// elements that are content on their own, without any text
var mediaTags = map[string]bool{"img": true, "picture": true, "video": true, "audio": true, "iframe": true, "canvas": true}

// ExtractDocument returns the gist of a page: language, title and description
// from the head, and the outline of the body. The outline holds the landmarks
// (the visible main, the page's header and footer, navs), articles, sections,
// asides and headings in document order. ARIA roles count as the elements they
// stand for: role="main", "banner", "contentinfo", "navigation", "article",
// "region", "complementary" and "heading" with aria-level; role="none" or
// "presentation" removes an element's semantics, other roles leave them as
// they are. A header or footer inside an article, aside, main, nav or section
// belongs to that element and is transparent, like all other elements: their
// semantic descendants become children of the nearest container. Hidden
// subtrees (hidden, aria-hidden="true") are left out.
//
// Divs are the fallback for pages without semantic containers, a div becomes a
// container when it has a heading of its own next to other content. Wrapper
// chains collapse to one div: a container whose only child is a div takes over
// that div's children (a semantic container always wins over the div), and the
// div's selector points to the tightest box of the chain that still holds all
// of the unit's content.
//
// Headings carry their accessible name as Text, containers the name their
// author gave them as Label, navs their links. With Options.Content the
// containers also hold their content, see addContent.
func ExtractDocument(htmlBytes []byte, options Options) (doc *Document, err error) {
	root, err := html.Parse(bytes.NewReader(htmlBytes))
	if err != nil {
		return nil, err
	}
	x := &extractor{ids: map[string]*html.Node{}, main: findMain(root)}
	x.collectIDs(root)
	doc = &Document{Body: []DocumentElement{}}
	if htmlNode := findElement(root, "html"); htmlNode != nil {
		doc.Lang = strings.TrimSpace(attr(htmlNode, "lang"))
	}
	if head := findElement(root, "head"); head != nil {
		if title := findElement(head, "title"); title != nil {
			doc.Title = normalize(textContent(title))
		}
		doc.Description = description(head)
	}
	if body := findElement(root, "body"); body != nil {
		doc.Body = append(doc.Body, x.containerChildren(body)...)
		if options.Content {
			doc.Body = x.addContent(body, doc.Body)
		}
	}
	forgetNodes(doc.Body)
	return doc, nil
}

// Main is the main landmark of the outline, nil if the page has none
func (d *Document) Main() *DocumentElement {
	var find func(elements []DocumentElement) *DocumentElement
	find = func(elements []DocumentElement) *DocumentElement {
		for i := range elements {
			if elements[i].ElementType == ElementTypeMain {
				return &elements[i]
			}
			if m := find(elements[i].Children); m != nil {
				return m
			}
		}
		return nil
	}
	return find(d.Body)
}

type extractor struct {
	// elements by id, for aria-labelledby
	ids map[string]*html.Node
	// main is the visible main of the page, other mains are transparent
	main *html.Node
}

func (x *extractor) collectIDs(n *html.Node) {
	if n.Type == html.ElementNode {
		if id := attr(n, "id"); id != "" {
			if _, ok := x.ids[id]; !ok {
				x.ids[id] = n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		x.collectIDs(c)
	}
}

// findMain returns the first <main> or role="main" outside hidden subtrees, a
// page may have more than one, but only one is visible
func findMain(n *html.Node) *html.Node {
	if n.Type == html.ElementNode {
		if skipTags[n.Data] {
			return nil
		}
		if isHidden(n) {
			return nil
		}
		if isMain(n) {
			return n
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if m := findMain(c); m != nil {
			return m
		}
	}
	return nil
}

func (x *extractor) extractChildren(n *html.Node) []DocumentElement {
	var elements []DocumentElement
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || skipTags[c.Data] || isHidden(c) {
			continue
		}
		if c == x.main {
			elements = append(elements, DocumentElement{
				ElementType: ElementTypeMain,
				node:        c,
				Selector:    locator(c),
				Label:       x.label(c),
				Children:    x.containerChildren(c),
			})
			continue
		}
		elementType, ok := elementTypeOf(c)
		switch {
		case ok && isHeading(elementType):
			elements = append(elements, DocumentElement{ElementType: elementType, node: c, Selector: locator(c), Text: x.headingName(c)})
		case ok:
			element := DocumentElement{
				ElementType: elementType,
				node:        c,
				Selector:    locator(c),
				Label:       x.label(c),
				Children:    x.containerChildren(c),
			}
			if elementType == ElementTypeNav {
				element.Links = x.links(c)
			}
			elements = append(elements, element)
		case c.Data == string(ElementTypeDiv) && !isPresentational(c):
			children := x.containerChildren(c)
			if isDivContainer(c, children) {
				box := tightest(c)
				elements = append(elements, DocumentElement{ElementType: ElementTypeDiv, node: box, Selector: locator(box), Label: x.label(box), Children: children})
			} else {
				elements = append(elements, children...)
			}
		default:
			elements = append(elements, x.extractChildren(c)...)
		}
	}
	return elements
}

// containerChildren are the children of a container: a single div child is
// absorbed, and a leading block of headings is unwrapped like a <header>
func (x *extractor) containerChildren(n *html.Node) []DocumentElement {
	children := x.extractChildren(n)
	for len(children) == 1 && children[0].ElementType == ElementTypeDiv {
		children = children[0].Children
	}
	if isHeaderBlock(children) {
		children = append(append([]DocumentElement{}, children[0].Children...), children[1:]...)
	}
	return children
}

// elementTypeOf is the outline type of n, from its role or its tag - the main
// is no type, there is only one, see extractor.main
func elementTypeOf(n *html.Node) (ElementType, bool) {
	for _, role := range strings.Fields(attr(n, "role")) {
		switch role {
		case "none", "presentation":
			return "", false
		case "heading":
			return headingType(n, 2), true
		}
		if t, ok := roleTypes[role]; ok {
			return t, true
		}
	}
	t, ok := elementTypes[n.Data]
	if ok && isHeading(t) {
		return headingType(n, int(t[1]-'0')), true
	}
	if (t == ElementTypeHeader || t == ElementTypeFooter) && isScoped(n) {
		return "", false
	}
	return t, ok
}

// isScoped reports that n is inside an element its header or footer would belong to
func isScoped(n *html.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type != html.ElementNode {
			continue
		}
		if landmarkScopes[p.Data] {
			return true
		}
		for _, role := range strings.Fields(attr(p, "role")) {
			if landmarkScopes["role:"+role] {
				return true
			}
		}
	}
	return false
}

func isMain(n *html.Node) bool {
	return n.Data == "main" || hasRole(n, "main")
}

// isHidden reports a subtree that is no part of the page for a reader
func isHidden(n *html.Node) bool {
	return hasAttr(n, "hidden") || attr(n, "aria-hidden") == "true"
}

// headingType is the heading of level aria-level, or of the given level
func headingType(n *html.Node, level int) ElementType {
	if l, err := strconv.Atoi(strings.TrimSpace(attr(n, "aria-level"))); err == nil && l > 0 {
		level = min(l, 6)
	}
	return ElementType("h" + strconv.Itoa(level))
}

func isPresentational(n *html.Node) bool {
	return hasRole(n, "none") || hasRole(n, "presentation")
}

// isDivContainer: the div has a heading of its own and groups it with other
// items or content - a div around nothing but a heading is just a wrapper
func isDivContainer(n *html.Node, children []DocumentElement) bool {
	hasHeading := false
	for _, c := range children {
		if isHeading(c.ElementType) {
			hasHeading = true
			break
		}
	}
	return hasHeading && (len(children) > 1 || hasContent(n))
}

// isHeaderBlock: the first child is a div of headings only, introducing what
// follows - unless all children are such divs of the same rank, like a grid of
// cards
func isHeaderBlock(children []DocumentElement) bool {
	if len(children) < 2 || !isHeadingsOnly(children[0]) {
		return false
	}
	rank := children[0].Children[0].ElementType
	for _, c := range children[1:] {
		if !isHeadingsOnly(c) || c.Children[0].ElementType > rank {
			return true
		}
	}
	return false
}

func isHeadingsOnly(e DocumentElement) bool {
	if e.ElementType != ElementTypeDiv || len(e.Children) == 0 {
		return false
	}
	for _, c := range e.Children {
		if !isHeading(c.ElementType) {
			return false
		}
	}
	return true
}

// tightest is the innermost div of the wrapper chain from n that still holds
// everything n holds: step into a div as long as it is the only child with
// content, so layout wrappers drop off, but nothing of the unit (an image next
// to the heading, a date) is cut off
func tightest(n *html.Node) *html.Node {
	for {
		var inner *html.Node
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			switch c.Type {
			case html.TextNode:
				if strings.TrimSpace(c.Data) != "" {
					return n
				}
			case html.ElementNode:
				if skipTags[c.Data] || isBlank(c) {
					continue
				}
				if inner != nil || c.Data != string(ElementTypeDiv) {
					return n
				}
				inner = c
			}
		}
		if inner == nil {
			return n
		}
		n = inner
	}
}

// isBlank reports that n has no visible content and no outline elements below
func isBlank(n *html.Node) bool {
	if isHidden(n) {
		return true
	}
	if _, ok := elementTypeOf(n); ok || mediaTags[n.Data] || isMain(n) {
		return false
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch c.Type {
		case html.TextNode:
			if strings.TrimSpace(c.Data) != "" {
				return false
			}
		case html.ElementNode:
			if !skipTags[c.Data] && !isBlank(c) {
				return false
			}
		}
	}
	return true
}

// hasContent reports visible text or media below n, outside of headings
func hasContent(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch c.Type {
		case html.TextNode:
			if strings.TrimSpace(c.Data) != "" {
				return true
			}
		case html.ElementNode:
			if t, _ := elementTypeOf(c); skipTags[c.Data] || isHidden(c) || isHeading(t) {
				continue
			}
			if mediaTags[c.Data] || hasContent(c) {
				return true
			}
		}
	}
	return false
}

func isHeading(t ElementType) bool {
	return len(t) == 2 && t[0] == 'h'
}

// label is the accessible name the author gave n: the names of the elements
// referenced by aria-labelledby, or aria-label
func (x *extractor) label(n *html.Node) string {
	var names []string
	for _, id := range strings.Fields(attr(n, "aria-labelledby")) {
		if ref, ok := x.ids[id]; ok {
			name := normalize(attr(ref, "aria-label"))
			if name == "" {
				name = contentName(ref)
			}
			names = append(names, name)
		}
	}
	if name := normalize(strings.Join(names, " ")); name != "" {
		return name
	}
	return normalize(attr(n, "aria-label"))
}

// headingName is the accessible name of a heading: its label, its content or
// its title
func (x *extractor) headingName(n *html.Node) string {
	if name := x.label(n); name != "" {
		return name
	}
	if name := contentName(n); name != "" {
		return name
	}
	return normalize(attr(n, "title"))
}

// contentName is the accessible name n gets from its content: text, alt texts
// and the aria-label or title of descendants, without hidden subtrees
func contentName(n *html.Node) string {
	sb := &strings.Builder{}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			switch c.Type {
			case html.TextNode:
				sb.WriteString(c.Data)
				sb.WriteByte(' ')
			case html.ElementNode:
				if skipTags[c.Data] || hasAttr(c, "hidden") || attr(c, "aria-hidden") == "true" {
					continue
				}
				name := normalize(attr(c, "aria-label"))
				isImage := c.Data == "img" || c.Data == "area" || (c.Data == "input" && attr(c, "type") == "image")
				if name == "" && isImage && hasAttr(c, "alt") {
					// alt="" marks a decorative image, it has no name at all
					sb.WriteString(normalize(attr(c, "alt")) + " ")
					continue
				}
				if name == "" {
					name = contentName(c)
				}
				if name == "" {
					name = normalize(attr(c, "title"))
				}
				sb.WriteString(name)
				sb.WriteByte(' ')
			}
		}
	}
	walk(n)
	return normalize(sb.String())
}

// links are the links below n, with their accessible names
func (x *extractor) links(n *html.Node) []Link {
	var links []Link
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode || skipTags[c.Data] || isHidden(c) {
				continue
			}
			if c.Data == "a" && hasAttr(c, "href") {
				name := x.label(c)
				if name == "" {
					name = contentName(c)
				}
				if name == "" {
					name = normalize(attr(c, "title"))
				}
				links = append(links, Link{Text: name, Href: strings.TrimSpace(attr(c, "href")), Selector: locator(c)})
				continue
			}
			walk(c)
		}
	}
	walk(n)
	return links
}

// description is the content of the description meta tag
func description(head *html.Node) string {
	for c := head.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "meta" && strings.EqualFold(attr(c, "name"), "description") {
			return normalize(attr(c, "content"))
		}
	}
	return ""
}

// findElement is the first element with the tag below n, depth first
func findElement(n *html.Node, tag string) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		if c.Data == tag {
			return c
		}
		if found := findElement(c, tag); found != nil {
			return found
		}
	}
	return nil
}

func textContent(n *html.Node) string {
	sb := &strings.Builder{}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			sb.WriteString(c.Data)
		}
	}
	return sb.String()
}

func normalize(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// locator is a CSS path to n, usable with document.querySelector, the same
// scheme as htmlschema's locators
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

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func hasRole(n *html.Node, role string) bool {
	for _, r := range strings.Fields(attr(n, "role")) {
		if r == role {
			return true
		}
	}
	return false
}
