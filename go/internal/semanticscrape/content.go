package semanticscrape

import (
	"strings"

	"golang.org/x/net/html"
)

// inline elements continue the text around them, all other elements end it
var inlineTags = map[string]bool{
	"a": true, "abbr": true, "b": true, "bdi": true, "bdo": true, "cite": true, "code": true, "data": true,
	"dfn": true, "em": true, "font": true, "i": true, "kbd": true, "label": true, "mark": true, "q": true,
	"s": true, "samp": true, "small": true, "span": true, "strong": true, "sub": true, "sup": true,
	"time": true, "u": true, "var": true, "wbr": true,
}

// controls and drawings are no content
var noContentTags = map[string]bool{
	"button": true, "input": true, "select": true, "textarea": true, "option": true, "svg": true, "math": true,
}

// addContent puts the content of n between its children in reading order -
// after the content of the children's own subtrees. Navs get none, their links
// are their content.
func (x *extractor) addContent(n *html.Node, children []DocumentElement) []DocumentElement {
	return x.contentOf(n, children, nil)
}

// contentOf is addContent, without the subtree skip
func (x *extractor) contentOf(n *html.Node, children []DocumentElement, skip *html.Node) []DocumentElement {
	for i := range children {
		child := &children[i]
		switch {
		case child.ElementType == ElementTypeNav:
		case isHeading(child.ElementType):
			child.Links = x.headingLinks(child.node)
		default:
			child.Children = x.addContent(child.node, child.Children)
		}
	}
	w := &contentWalker{x: x, container: n, skip: skip, children: children, at: map[*html.Node]int{}, reached: make([]bool, len(children))}
	for i, c := range children {
		w.at[c.node] = i
	}
	w.walk(n)
	w.flush()
	// children the walk did not pass are kept, at the end
	for i, reached := range w.reached {
		if !reached {
			w.out = append(w.out, children[i])
		}
	}
	return w.out
}

// contentWalker walks a container in document order: it passes its children
// on and collects the content between them
type contentWalker struct {
	x         *extractor
	container *html.Node
	// skip is a subtree that is no content, a figure's caption
	skip     *html.Node
	children []DocumentElement
	// at is the index of the child by its node
	at      map[*html.Node]int
	reached []bool
	out     []DocumentElement
	// the text being collected, its links and its first node
	text  strings.Builder
	links []Link
	first *html.Node
}

func (w *contentWalker) walk(n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch c.Type {
		case html.TextNode:
			w.addText(c.Data, c)
		case html.ElementNode:
			w.element(c)
		}
	}
}

func (w *contentWalker) element(c *html.Node) {
	if skipTags[c.Data] || noContentTags[c.Data] || isHidden(c) || c == w.skip {
		return
	}
	if i, ok := w.at[c]; ok {
		w.flush()
		w.out = append(w.out, w.children[i])
		w.reached[i] = true
		return
	}
	switch {
	case w.holdsChild(c):
		// a teaser link or figure around a heading: the heading is no content
		w.flush()
		w.walk(c)
		w.flush()
	case c.Data == "img":
		w.flush()
		if img, ok := image(c); ok {
			w.out = append(w.out, img)
		}
	case c.Data == "figure":
		w.flush()
		w.out = append(w.out, w.x.figure(c))
	case isList(c):
		w.flush()
		// a list of empty items is decoration
		if list := w.x.list(c); len(list.Children) > 0 {
			w.out = append(w.out, list)
		}
	case c.Data == "table" && !isPresentational(c):
		w.flush()
		w.out = append(w.out, w.x.table(c))
	case c.Data == "a" && hasAttr(c, "href"):
		link := w.x.link(c)
		if !hasText(c) {
			// an image link: the link belongs to its images
			w.flush()
			for _, img := range images(c) {
				img.Links = []Link{link}
				w.out = append(w.out, img)
			}
			return
		}
		w.addText(link.Text, c)
		w.links = append(w.links, link)
	case c.Data == "br":
		w.addText(" ", c)
	case inlineTags[c.Data]:
		w.walk(c)
	default:
		w.flush()
		w.walk(c)
		w.flush()
	}
}

// holdsChild reports one of the children below n
func (w *contentWalker) holdsChild(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if _, ok := w.at[c]; ok || (c.Type == html.ElementNode && w.holdsChild(c)) {
			return true
		}
	}
	return false
}

func (w *contentWalker) addText(text string, n *html.Node) {
	if w.first == nil && strings.TrimSpace(text) != "" {
		w.first = n
	}
	w.text.WriteString(text)
}

// flush ends the text being collected
func (w *contentWalker) flush() {
	if text := normalize(w.text.String()); text != "" {
		w.out = append(w.out, DocumentElement{
			ElementType: ElementTypeText,
			Selector:    locator(w.block(w.first)),
			Text:        text,
			Links:       w.links,
		})
	}
	w.text.Reset()
	w.links = nil
	w.first = nil
}

// block is the element a text belongs to: the nearest ancestor of its first
// node that is no inline element, at most the container
func (w *contentWalker) block(n *html.Node) *html.Node {
	if n.Type == html.TextNode || inlineTags[n.Data] {
		n = n.Parent
	}
	for n != w.container && n.Parent != nil && inlineTags[n.Data] {
		n = n.Parent
	}
	return n
}

// figure is a figure with its caption as text and its content
func (x *extractor) figure(n *html.Node) DocumentElement {
	figure := DocumentElement{ElementType: ElementTypeFigure, Selector: locator(n), Label: x.label(n)}
	var caption *html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "figcaption" && !isHidden(c) {
			caption = c
			break
		}
	}
	if caption != nil {
		figure.Text = contentName(caption)
	}
	// the caption is the figure's text, not content of it
	figure.Children = x.contentOf(n, nil, caption)
	return figure
}

// isList reports ul, ol, dl and role="list"
func isList(n *html.Node) bool {
	return n.Data == "ul" || n.Data == "ol" || n.Data == "dl" || hasRole(n, "list")
}

// list is a list with its items: li of ul, ol and role="list" (items with
// role="listitem"), dt and dd of dl - empty items are left out
func (x *extractor) list(n *html.Node) DocumentElement {
	list := DocumentElement{ElementType: ElementTypeUnorderedList, Selector: locator(n), Label: x.label(n)}
	switch {
	case hasRole(n, "list"):
	case n.Data == "ol":
		list.ElementType = ElementTypeOrderedList
	case n.Data == "dl":
		list.ElementType = ElementTypeDefinitions
	}
	add := func(item DocumentElement) {
		if item.Text != "" || len(item.Links) > 0 || len(item.Children) > 0 {
			list.Children = append(list.Children, item)
		}
	}
	var items func(n *html.Node)
	items = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode || skipTags[c.Data] || isHidden(c) {
				continue
			}
			switch {
			case c.Data == "li" || hasRole(c, "listitem"):
				add(x.listItem(c, ElementTypeListItem))
			case list.ElementType == ElementTypeDefinitions && c.Data == "dt":
				add(x.listItem(c, ElementTypeTerm))
			case list.ElementType == ElementTypeDefinitions && c.Data == "dd":
				add(x.listItem(c, ElementTypeDefinition))
			case c.Data == "div":
				// dl groups its terms in divs
				items(c)
			}
		}
	}
	items(n)
	return list
}

// listItem is an item with its leading text as its text, the rest of its
// content are its children
func (x *extractor) listItem(n *html.Node, elementType ElementType) DocumentElement {
	item := DocumentElement{ElementType: elementType, Selector: locator(n)}
	content := x.contentOf(n, nil, nil)
	if len(content) > 0 && content[0].ElementType == ElementTypeText {
		item.Text = content[0].Text
		item.Links = content[0].Links
		content = content[1:]
	}
	if len(content) > 0 {
		item.Children = content
	}
	return item
}

// table is a table with its caption as text, its rows and their cells - the
// text and the links of each cell
func (x *extractor) table(n *html.Node) DocumentElement {
	table := DocumentElement{ElementType: ElementTypeTable, Selector: locator(n), Label: x.label(n)}
	var rows func(n *html.Node)
	rows = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode || skipTags[c.Data] || isHidden(c) {
				continue
			}
			switch c.Data {
			case "caption":
				table.Text = contentName(c)
			case "thead", "tbody", "tfoot":
				rows(c)
			case "tr":
				row := DocumentElement{ElementType: ElementTypeRow, Selector: locator(c)}
				for cell := c.FirstChild; cell != nil; cell = cell.NextSibling {
					if cell.Type != html.ElementNode || isHidden(cell) || (cell.Data != "th" && cell.Data != "td") {
						continue
					}
					row.Children = append(row.Children, DocumentElement{
						ElementType: ElementType(cell.Data),
						Selector:    locator(cell),
						Text:        contentName(cell),
						Links:       x.links(cell),
					})
				}
				table.Children = append(table.Children, row)
			}
		}
	}
	rows(n)
	return table
}

// headingLinks are the links of a heading: the links in it, or the link
// around it - a teaser
func (x *extractor) headingLinks(n *html.Node) []Link {
	if links := x.links(n); len(links) > 0 {
		return links
	}
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && p.Data == "a" && hasAttr(p, "href") {
			return []Link{x.link(p)}
		}
	}
	return nil
}

// link is a link with its accessible name
func (x *extractor) link(a *html.Node) Link {
	name := x.label(a)
	if name == "" {
		name = contentName(a)
	}
	if name == "" {
		name = normalize(attr(a, "title"))
	}
	return Link{Text: name, Href: strings.TrimSpace(attr(a, "href")), Selector: locator(a)}
}

// image is an img with its accessible name - alt="" marks a decorative one -
// and its source: src, or for a lazy loading placeholder data-src or the
// first candidate of the srcset
func image(n *html.Node) (DocumentElement, bool) {
	src := strings.TrimSpace(attr(n, "src"))
	for _, candidate := range []string{attr(n, "data-src"), firstCandidate(attr(n, "srcset")), firstCandidate(attr(n, "data-srcset"))} {
		if src != "" && !strings.HasPrefix(src, "data:") {
			break
		}
		if candidate = strings.TrimSpace(candidate); candidate != "" {
			src = candidate
		}
	}
	if src == "" {
		return DocumentElement{}, false
	}
	name := normalize(attr(n, "aria-label"))
	if name == "" {
		name = normalize(attr(n, "alt"))
	}
	return DocumentElement{ElementType: ElementTypeImage, Selector: locator(n), Text: name, Src: src}, true
}

// images are the visible images below n
func images(n *html.Node) []DocumentElement {
	var result []DocumentElement
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || skipTags[c.Data] || isHidden(c) {
			continue
		}
		if c.Data == "img" {
			if img, ok := image(c); ok {
				result = append(result, img)
			}
			continue
		}
		result = append(result, images(c)...)
	}
	return result
}

func firstCandidate(srcset string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(srcset), ",")
	url, _, _ := strings.Cut(strings.TrimSpace(first), " ")
	return url
}

// hasText reports visible text below n
func hasText(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch c.Type {
		case html.TextNode:
			if strings.TrimSpace(c.Data) != "" {
				return true
			}
		case html.ElementNode:
			if !skipTags[c.Data] && !noContentTags[c.Data] && !isHidden(c) && hasText(c) {
				return true
			}
		}
	}
	return false
}

// forgetNodes drops the page from the elements, the extraction is done
func forgetNodes(elements []DocumentElement) {
	for i := range elements {
		elements[i].node = nil
		forgetNodes(elements[i].Children)
	}
}
