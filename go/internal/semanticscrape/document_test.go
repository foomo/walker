package semanticscrape

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "rewrite the golden outlines in test_data")

func readTestData(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("test_data", name))
	require.NoError(t, err)
	return b
}

// mainOf is the outline below the main of the document, the main must exist
func mainOf(t *testing.T, doc *Document) []DocumentElement {
	t.Helper()
	main := doc.Main()
	require.NotNil(t, main, "the document has no main")
	return main.Children
}

func extract(t *testing.T, htmlBytes []byte) *Document {
	t.Helper()
	t.Helper()
	return extractWith(t, htmlBytes, Options{})
}

func extractContent(t *testing.T, htmlBytes []byte) *Document {
	t.Helper()
	return extractWith(t, htmlBytes, Options{Content: true})
}

func extractWith(t *testing.T, htmlBytes []byte, options Options) *Document {
	t.Helper()
	doc, err := ExtractDocument(htmlBytes, options)
	require.NoError(t, err)
	require.NotNil(t, doc)
	return doc
}

// outline renders the elements one per line, indented by depth, without
// selectors: "section [label]", "h2 text", "img alt <src>", links as
// "- text (href)"
func outline(elements []DocumentElement) string {
	sb := &strings.Builder{}
	var walk func(elements []DocumentElement, depth int)
	walk = func(elements []DocumentElement, depth int) {
		for _, e := range elements {
			sb.WriteString(strings.Repeat("  ", depth) + string(e.ElementType))
			if e.Label != "" {
				sb.WriteString(" [" + e.Label + "]")
			}
			if e.Text != "" {
				sb.WriteString(" " + e.Text)
			}
			if e.Src != "" {
				sb.WriteString(" <" + e.Src + ">")
			}
			sb.WriteString("\n")
			for _, l := range e.Links {
				sb.WriteString(strings.Repeat("  ", depth+1) + "- " + l.Text + " (" + l.Href + ")\n")
			}
			walk(e.Children, depth+1)
		}
	}
	walk(elements, 0)
	return sb.String()
}

func countType(elements []DocumentElement, elementType ElementType) int {
	count := 0
	for _, e := range elements {
		if e.ElementType == elementType {
			count++
		}
		count += countType(e.Children, elementType)
	}
	return count
}

// outlineWithClasses is outline, with the class of each div: div.outer
func outlineWithClasses(t *testing.T, htmlBytes []byte, elements []DocumentElement) string {
	t.Helper()
	page, err := goquery.NewDocumentFromReader(bytes.NewReader(htmlBytes))
	require.NoError(t, err)
	var withClasses func(elements []DocumentElement) []DocumentElement
	withClasses = func(elements []DocumentElement) []DocumentElement {
		result := make([]DocumentElement, len(elements))
		for i, e := range elements {
			if class := page.Find(e.Selector).AttrOr("class", ""); e.ElementType == ElementTypeDiv && class != "" {
				e.ElementType = ElementType("div." + class)
			}
			e.Children = withClasses(e.Children)
			result[i] = e
		}
		return result
	}
	return outline(withClasses(elements))
}

// assertSelectors checks that every selector finds exactly one element of its
// type in the page, or one with ARIA attributes that give it the type
func assertSelectors(t *testing.T, htmlBytes []byte, elements []DocumentElement) int {
	t.Helper()
	page, err := goquery.NewDocumentFromReader(bytes.NewReader(htmlBytes))
	require.NoError(t, err)
	count := 0
	var walk func(elements []DocumentElement)
	walk = func(elements []DocumentElement) {
		for _, e := range elements {
			count++
			sel := page.Find(e.Selector)
			if assert.Equal(t, 1, sel.Length(), e.Selector) {
				_, hasRole := sel.Attr("role")
				_, hasLevel := sel.Attr("aria-level")
				// a text is a block of any kind
				if !hasRole && !hasLevel && e.ElementType != ElementTypeText {
					assert.Equal(t, string(e.ElementType), goquery.NodeName(sel), e.Selector)
				}
			}
			for _, l := range e.Links {
				link := page.Find(l.Selector)
				if assert.Equal(t, 1, link.Length(), l.Selector) {
					assert.Equal(t, "a", goquery.NodeName(link), l.Selector)
					assert.Equal(t, l.Href, strings.TrimSpace(link.AttrOr("href", "")), l.Selector)
				}
			}
			walk(e.Children)
		}
	}
	walk(elements)
	return count
}

func TestExtractDocument_Sourdough(t *testing.T) {
	htmlBytes := readTestData(t, "keeping-sourdough-alive.html")
	doc := extract(t, htmlBytes)

	const article = "html > body > main > article"
	expected := []DocumentElement{
		{
			ElementType: ElementTypeArticle,
			Selector:    article,
			Children: []DocumentElement{
				{ElementType: ElementTypeH1, Selector: article + " > header > h1", Text: "Keeping a sourdough starter alive"},
				{
					ElementType: ElementTypeSection,
					Selector:    article + " > section:nth-of-type(1)",
					Label:       "Feeding",
					Children: []DocumentElement{
						{ElementType: ElementTypeH2, Selector: article + " > section:nth-of-type(1) > h2", Text: "Feeding"},
					},
				},
				{
					ElementType: ElementTypeSection,
					Selector:    article + " > section:nth-of-type(2)",
					Label:       "Signs it’s healthy",
					Children: []DocumentElement{
						{ElementType: ElementTypeH2, Selector: article + " > section:nth-of-type(2) > h2", Text: "Signs it’s healthy"},
					},
				},
				{
					ElementType: ElementTypeSection,
					Selector:    article + " > section:nth-of-type(3)",
					Label:       "Storing it",
					Children: []DocumentElement{
						{ElementType: ElementTypeH2, Selector: article + " > section:nth-of-type(3) > h2", Text: "Storing it"},
					},
				},
			},
		},
		{
			ElementType: ElementTypeAside,
			Selector:    "html > body > main > aside",
			Label:       "Related guides",
			Children: []DocumentElement{
				{ElementType: ElementTypeH2, Selector: "html > body > main > aside > h2", Text: "Related guides"},
			},
		},
	}
	assert.Equal(t, expected, mainOf(t, doc))
	assert.Equal(t, 10, assertSelectors(t, htmlBytes, mainOf(t, doc)))
}

// the news homepage: deeply wrapped, sections in sections, no h1, h3 without h2
func TestExtractDocument_NewsHome(t *testing.T) {
	htmlBytes := readTestData(t, "news-portal-home.html")
	doc := extract(t, htmlBytes)

	actual := outline(doc.Body)
	golden := filepath.Join("test_data", "news-portal-home.outline.txt")
	if *update {
		require.NoError(t, os.WriteFile(golden, []byte(actual), 0o644))
	}
	expected, err := os.ReadFile(golden)
	require.NoError(t, err, "run go test ./semanticscrape -update to create it")
	assert.Equal(t, string(expected), actual)

	// all semantic elements below main are in the tree, nothing else
	page, err := goquery.NewDocumentFromReader(bytes.NewReader(htmlBytes))
	require.NoError(t, err)
	hidden := "[hidden], [hidden] *, [aria-hidden=true], [aria-hidden=true] *"
	total := page.Find("main").Find("article, section, aside, nav, h1, h2, h3, h4, h5, h6").Not(hidden).Length()
	assert.Equal(t, total+countType(mainOf(t, doc), ElementTypeDiv), assertSelectors(t, htmlBytes, mainOf(t, doc)))
	assert.Equal(t, "News portal home", doc.Title)

	content := extractContent(t, htmlBytes)
	actualContent := outline(content.Body)
	goldenContent := filepath.Join("test_data", "news-portal-home.content.txt")
	if *update {
		require.NoError(t, os.WriteFile(goldenContent, []byte(actualContent), 0o644))
	}
	expectedContent, err := os.ReadFile(goldenContent)
	require.NoError(t, err, "run go test ./semanticscrape -update to create it")
	assert.Equal(t, string(expectedContent), actualContent)
	assert.Equal(t, actual, outline(withoutContent(content.Body)), "content adds to the structure, it does not change it")
	assertSelectors(t, htmlBytes, content.Body)
	assert.NotContains(t, actual, "h1")
	assert.NotContains(t, actual, "<", "noscript markup leaked into heading text")
}

func TestExtractDocument(t *testing.T) {
	tests := []outlineTest{
		{
			name:     "no main",
			html:     `<body><article><h1>outside</h1></article></body>`,
			expected: "-",
		},
		{
			name:     "empty main",
			html:     `<main><div><p>no structure</p></div></main>`,
			expected: "",
		},
		{
			name:     "hidden main is skipped",
			html:     `<main hidden><h1>hidden</h1></main><main><h1>visible</h1></main>`,
			expected: "h1 visible\n",
		},
		{
			name: "wrappers, header and footer of a section are transparent, its nav is not",
			html: `<main><div class="a"><div><section><header><div><h2>Title</h2></div></header>
				<nav><aside><h3>Nav</h3></aside></nav><footer><h4>Foot</h4></footer></section></div></div></main>`,
			expected: "section\n  h2 Title\n  nav\n    aside\n      h3 Nav\n  h4 Foot\n",
		},
		{
			name:     "document order across wrappers",
			html:     `<main><div><h2>a</h2></div><article></article><div><div><h2>b</h2></div></div><aside></aside></main>`,
			expected: "h2 a\narticle\nh2 b\naside\n",
		},
		{
			name: "heading text is normalized, scripts and templates are skipped",
			html: `<main><h2>
				<span class="flag"><svg><use href="#i"></use></svg></span>
				<span>Spaced   <b>out</b></span><script>var x = 1</script><noscript><img src="x.jpg"></noscript></h2>
				<template><section><h2>template</h2></section></template></main>`,
			expected: "h2 Spaced out\n",
		},
		{
			name:     "nested articles",
			html:     `<main><article><article><h3>inner</h3></article><h2>after</h2></article></main>`,
			expected: "article\n  article\n    h3 inner\n  h2 after\n",
		},
	}
	runOutlineTests(t, tests)
}

// div fallback: divs stand in for missing containers, one div per container
func TestExtractDocument_Divs(t *testing.T) {
	runOutlineTests(t, []outlineTest{
		{
			name:     "a div around a heading only is a wrapper",
			html:     `<main><div><div class="title"><h2>a</h2></div></div></main>`,
			expected: "h2 a\n",
		},
		{
			name:     "a div with a heading and content is a container",
			html:     `<main><h1>page</h1><div><h2>a</h2><p>text</p></div><div><h2>b</h2><img src="b.jpg"></div></main>`,
			expected: "h1 page\ndiv\n  h2 a\ndiv\n  h2 b\n",
		},
		{
			name:     "a div with a heading and other items is a container",
			html:     `<main><h1>page</h1><div><h2>a</h2><article></article></div></main>`,
			expected: "h1 page\ndiv\n  h2 a\n  article\n",
		},
		{
			name:     "a div without a heading of its own is a wrapper",
			html:     `<main><div class="grid"><div><h2>a</h2>text</div><div><h2>b</h2>text</div></div></main>`,
			expected: "div\n  h2 a\ndiv\n  h2 b\n",
		},
		{
			name:     "a wrapper chain collapses to its tightest div",
			html:     `<main><h1>page</h1><div class="outer"><div class="mid"><div class="inner"><h2>a</h2>text</div></div></div><p>x</p></main>`,
			expected: "h1 page\ndiv.inner\n  h2 a\n",
		},
		{
			name: "the tightest div keeps content next to the heading",
			html: `<main><h1>shop</h1><div class="cards">
				<div class="card"><div class="card__media"><img src="a.jpg"></div><div class="card__inner"><h2>a</h2><p>text</p></div></div>
				<div class="card"><div class="card__media"><img src="b.jpg"></div><div class="card__inner"><h2>b</h2><p>text</p></div></div>
				</div></main>`,
			expected: "h1 shop\ndiv.card\n  h2 a\ndiv.card\n  h2 b\n",
		},
		{
			name:     "the tightest div keeps text next to the chain",
			html:     `<main><h1>page</h1><div class="outer">date<div class="inner"><h2>a</h2>text</div></div></main>`,
			expected: "h1 page\ndiv.outer\n  h2 a\n",
		},
		{
			name:     "the tightest div ignores blank siblings",
			html:     `<main><h1>page</h1><div class="outer"><div class="spacer"></div><script>x()</script><div class="inner"><h2>a</h2>text</div><span> </span></div></main>`,
			expected: "h1 page\ndiv.inner\n  h2 a\n",
		},
		{
			name:     "the tightest div keeps empty outline elements",
			html:     `<main><h1>page</h1><div class="outer"><div class="inner"><h2>a</h2>text</div><article></article></div></main>`,
			expected: "h1 page\ndiv.outer\n  h2 a\n  article\n",
		},
		{
			name:     "the tightest div stops at other wrappers",
			html:     `<main><h1>page</h1><div class="outer"><a href="/a"><div class="inner"><h2>a</h2>text</div></a></div></main>`,
			expected: "h1 page\ndiv.outer\n  h2 a\n",
		},
		{
			name:     "a semantic container absorbs its only div",
			html:     `<main><article><div><div><h2>a</h2></div><p>text</p></div></article></main>`,
			expected: "article\n  h2 a\n",
		},
		{
			name:     "main absorbs its only div",
			html:     `<main><div class="page"><h1>a</h1><p>text</p></div></main>`,
			expected: "h1 a\n",
		},
		{
			name: "a leading heading block is unwrapped like a header",
			html: `<main><div class="post"><div class="head"><h1>post</h1><p>by someone</p></div>
				<div><h2>a</h2>text</div><div><h2>b</h2>text</div></div><aside><h2>related</h2></aside></main>`,
			expected: "div.post\n  h1 post\n  div\n    h2 a\n  div\n    h2 b\naside\n  h2 related\n",
		},
		{
			name:     "a heading block before articles is unwrapped",
			html:     `<main><section><div class="head"><h2>news</h2><a href="/more">more</a></div><article><h2>a</h2></article></section></main>`,
			expected: "section\n  h2 news\n  article\n    h2 a\n",
		},
		{
			name:     "cards of the same rank stay cards",
			html:     `<main><h1>shop</h1><div class="cards"><div class="card"><h2>a</h2>text</div><div class="card"><h2>b</h2>text</div></div></main>`,
			expected: "h1 shop\ndiv.card\n  h2 a\ndiv.card\n  h2 b\n",
		},
	})
}

func TestExtractDocument_DivSoup(t *testing.T) {
	semantic := extract(t, readTestData(t, "keeping-sourdough-alive.html"))
	htmlBytes := readTestData(t, "keeping-sourdough-alive-divs.html")
	divs := extract(t, htmlBytes)

	// the same outline, with divs instead of article, section and aside
	expected := outline(withoutLabels(mainOf(t, semantic)))
	for _, tag := range []string{"article", "section", "aside"} {
		expected = strings.ReplaceAll(expected, tag, "div")
	}
	assert.Equal(t, expected, outline(mainOf(t, divs)))
	assertSelectors(t, htmlBytes, mainOf(t, divs))

	// the tightest div of each wrapper chain is kept, without the layout columns
	assert.Equal(t, []string{"post__inner", "widget"}, classes(t, htmlBytes, mainOf(t, divs)))
	assert.Equal(t, []string{"block__wrap", "block__wrap", "block__wrap"}, classes(t, htmlBytes, mainOf(t, divs)[0].Children[1:]))
}

func withoutLabels(elements []DocumentElement) []DocumentElement {
	result := make([]DocumentElement, len(elements))
	for i, e := range elements {
		e.Label = ""
		e.Children = withoutLabels(e.Children)
		result[i] = e
	}
	return result
}

// the div soup again, with ARIA roles and labels: the same outline as the
// semantic page, landmarks, labels and links included
func TestExtractDocument_Aria(t *testing.T) {
	semantic := extract(t, readTestData(t, "keeping-sourdough-alive.html"))
	htmlBytes := readTestData(t, "keeping-sourdough-alive-aria.html")
	aria := extract(t, htmlBytes)
	assert.Equal(t, outline(semantic.Body), outline(aria.Body))
	assert.Equal(t, 10, assertSelectors(t, htmlBytes, mainOf(t, aria)))
	assertSelectors(t, htmlBytes, aria.Body)
}

func TestExtractDocument_Page(t *testing.T) {
	htmlBytes := readTestData(t, "keeping-sourdough-alive.html")
	doc := extract(t, htmlBytes)
	assert.Equal(t, "en", doc.Lang)
	assert.Equal(t, "Keeping a sourdough starter alive – Crumb Notes", doc.Title)
	assert.Equal(t, "A short guide to feeding and storing a sourdough starter.", doc.Description)
	assert.Equal(t, `header
  nav [Main]
    - Home (/)
    - Guides (/guides)
    - About (/about)
main
  article
    h1 Keeping a sourdough starter alive
    section [Feeding]
      h2 Feeding
    section [Signs it’s healthy]
      h2 Signs it’s healthy
    section [Storing it]
      h2 Storing it
  aside [Related guides]
    h2 Related guides
footer
  nav [Footer]
    - Privacy (/privacy)
    - Contact (/contact)
`, outline(doc.Body))
	assertSelectors(t, htmlBytes, doc.Body)

	// the div soup has no landmarks but its main
	divs := extract(t, readTestData(t, "keeping-sourdough-alive-divs.html"))
	require.Len(t, divs.Body, 1)
	assert.Equal(t, ElementTypeMain, divs.Body[0].ElementType)
}

func TestExtractDocument_Content(t *testing.T) {
	htmlBytes := readTestData(t, "keeping-sourdough-alive.html")
	doc := extractContent(t, htmlBytes)
	assert.Equal(t, `text Skip to content
  - Skip to content (#main-content)
header
  text Crumb Notes
    - Crumb Notes (/)
  nav [Main]
    - Home (/)
    - Guides (/guides)
    - About (/about)
main
  article
    h1 Keeping a sourdough starter alive
    text Feed it, watch it, and don’t panic.
    text October 21, 2026, by Lena Huber
      - Lena Huber (/authors/lena)
    section [Feeding]
      h2 Feeding
      text Discard half, then add equal weights of flour and water. Do this once a day at room temperature.
    section [Signs it’s healthy]
      h2 Signs it’s healthy
      ul
        li It doubles within 4–8 hours.
        li It smells tangy, not sharp.
        li The surface is full of bubbles.
      figure A starter at its peak, about six hours after feeding.
        img A jar of bubbly starter, risen to twice its marked height <starter.jpg>
    section [Storing it]
      h2 Storing it
      text In the fridge, a starter needs feeding only once a week. Let it warm up for a few hours before baking.
    text Filed under sourdough and basics.
      - sourdough (/tags/sourdough)
      - basics (/tags/basics)
  aside [Related guides]
    h2 Related guides
    ul
      li Your first loaf
        - Your first loaf (/guides/first-loaf)
      li Choosing flour
        - Choosing flour (/guides/flour)
footer
  text © 2026 Crumb Notes
  nav [Footer]
    - Privacy (/privacy)
    - Contact (/contact)
`, outline(doc.Body))
	assertSelectors(t, htmlBytes, doc.Body)

	// without the option the outline is the structure only
	plain := extract(t, htmlBytes)
	for contentType := range contentTypes {
		assert.Zero(t, countType(plain.Body, contentType), contentType)
	}
	assert.Nil(t, plain.Main().Children[0].Children[0].Links, "headings get their links with content only")

	// the structure is the same with content
	assert.Equal(t, outline(plain.Body), outline(withoutContent(doc.Body)))
}

// contentTypes are the element types of Options.Content
var contentTypes = map[ElementType]bool{
	ElementTypeText: true, ElementTypeImage: true, ElementTypeFigure: true,
	ElementTypeUnorderedList: true, ElementTypeOrderedList: true, ElementTypeDefinitions: true, ElementTypeTable: true,
}

// withoutContent drops the content and the links of headings
func withoutContent(elements []DocumentElement) []DocumentElement {
	var result []DocumentElement
	for _, e := range elements {
		switch {
		case contentTypes[e.ElementType]:
			continue
		case isHeading(e.ElementType):
			e.Links = nil
		}
		e.Children = withoutContent(e.Children)
		result = append(result, e)
	}
	return result
}

func TestExtractDocument_ContentRules(t *testing.T) {
	runContentTests(t, []outlineTest{
		{
			name:     "inline elements continue a text, blocks end it",
			html:     `<main><p>One <b>bold</b>, <time>today</time><br>next line</p><div>Two<div>Three</div>Four</div></main>`,
			expected: "main\n  text One bold, today next line\n  text Two\n  text Three\n  text Four\n",
		},
		{
			name:     "content between the elements, in reading order",
			html:     `<main>Intro<section><h2>a</h2><p>in a</p></section>Between<section><p>before</p><h2>b</h2></section>Outro</main>`,
			expected: "main\n  text Intro\n  section\n    h2 a\n    text in a\n  text Between\n  section\n    text before\n    h2 b\n  text Outro\n",
		},
		{
			name:     "links in a text",
			html:     `<main><p>Read <a href="/a">this</a> and <a href="/b" aria-label="that one">that</a>.</p><a href="/c">Standalone</a></main>`,
			expected: "main\n  text Read this and that one.\n    - this (/a)\n    - that one (/b)\n  text Standalone\n    - Standalone (/c)\n",
		},
		{
			name: "images, lazy ones and image links",
			html: `<main><img src="a.jpg" alt="A"><img src="data:image/gif;base64,R0" data-src="b.jpg" alt="">
				<img src="data:image/gif;base64,R0" srcset="c-1.jpg 1x, c-2.jpg 2x"><img alt="no source">
				<a href="/d"><picture><source srcset="d.webp"><img src="d.jpg" alt="D"></picture></a></main>`,
			expected: "main\n  img A <a.jpg>\n  img <b.jpg>\n  img <c-1.jpg>\n  img D <d.jpg>\n    - D (/d)\n",
		},
		{
			name:     "a figure with its caption",
			html:     `<main><figure><img src="a.jpg" alt="A"><figcaption>The <b>caption</b></figcaption><p>credit</p></figure></main>`,
			expected: "main\n  figure The caption\n    img A <a.jpg>\n    text credit\n",
		},
		{
			name: "teaser links belong to their headings",
			html: `<main><article><a href="/t"><img src="t.jpg" alt=""><h2>Teaser</h2><p>Summary</p></a></article>
				<article><h2><a href="/u">Inner</a></h2></article></main>`,
			// the link's name is all its content, as a screen reader reads it
			expected: "main\n  article\n    img <t.jpg>\n    h2 Teaser\n      - Teaser Summary (/t)\n    text Summary\n  article\n    h2 Inner\n      - Inner (/u)\n",
		},
		{
			name: "no content from controls, drawings, hidden subtrees and navs",
			html: `<main><button>Save</button><svg><text>chart</text></svg><p hidden>gone</p><span aria-hidden="true">icon</span>
				<input value="x"><nav><p>nav text</p><a href="/n">N</a></nav><p>kept</p></main>`,
			expected: "main\n  nav\n    - N (/n)\n  text kept\n",
		},
		{
			name: "lists",
			html: `<main><ul><li>One</li><li>Two <a href="/2">link</a><ol><li>Nested</li></ol></li><li> </li></ul>
				<ul><li></li></ul><div role="list"><div role="listitem">ARIA</div></div>
				<dl><dt>Term</dt><dd>Definition</dd><div><dt>Grouped</dt><dd>too</dd></div></dl></main>`,
			expected: "main\n  ul\n    li One\n    li Two link\n      - link (/2)\n      ol\n        li Nested\n  ul\n    li ARIA\n  dl\n    dt Term\n    dd Definition\n    dt Grouped\n    dd too\n",
		},
		{
			name:     "a list item with more than a text",
			html:     `<main><ul><li><p>Lead</p><img src="i.jpg" alt="I"><p>More</p></li><li><img src="j.jpg" alt="J"></li></ul></main>`,
			expected: "main\n  ul\n    li Lead\n      img I <i.jpg>\n      text More\n    li\n      img J <j.jpg>\n",
		},
		{
			name: "tables",
			html: `<main><table aria-label="Prices"><caption>Plans</caption><thead><tr><th>Plan</th><th>Price</th></tr></thead>
				<tbody><tr><td><a href="/basic">Basic</a></td><td>5 €</td></tr><tr hidden><td>gone</td></tr><tr><td>Pro <img src="p.png" alt="star"></td><td></td></tr></tbody></table></main>`,
			expected: "main\n  table [Prices] Plans\n    tr\n      th Plan\n      th Price\n    tr\n      td Basic\n        - Basic (/basic)\n      td 5 €\n    tr\n      td Pro star\n      td\n",
		},
		{
			name:     "layout tables and lists of teasers stay transparent",
			html:     `<main><table role="presentation"><tr><td>Cell text</td></tr></table><ul><li><article><h2>Teaser</h2></article></li></ul></main>`,
			expected: "main\n  text Cell text\n  article\n    h2 Teaser\n",
		},
		{
			name:     "content of a div container",
			html:     `<main><h1>page</h1><div class="card"><h2>a</h2><p>text</p></div></main>`,
			expected: "main\n  h1 page\n  div\n    h2 a\n    text text\n",
		},
	})
}

func TestExtractDocument_Head(t *testing.T) {
	doc := extract(t, []byte(`<html lang=" de "><head><title>
		A   title </title><meta name="Description" content=" Some  text "></head>
		<body><svg><title>not the title</title></svg></body></html>`))
	assert.Equal(t, "de", doc.Lang)
	assert.Equal(t, "A title", doc.Title)
	assert.Equal(t, "Some text", doc.Description)

	doc = extract(t, []byte(`<p>nothing</p>`))
	assert.Empty(t, doc.Lang+doc.Title+doc.Description)
	assert.NotNil(t, doc.Body, "an empty body encodes as []")
	assert.Empty(t, doc.Body)
	assert.Nil(t, doc.Main())
}

func TestExtractDocument_Landmarks(t *testing.T) {
	runBodyTests(t, []outlineTest{
		{
			name: "the page's header and footer are landmarks, an article's are not",
			html: `<body><header><h1>site</h1></header><main><article><header><h2>a</h2></header><p>text</p><footer>by</footer></article></main>
				<footer><p>imprint</p></footer></body>`,
			expected: "header\n  h1 site\nmain\n  article\n    h2 a\nfooter\n",
		},
		{
			name:     "a header in a div is still the page's",
			html:     `<body><div class="top"><header><h1>site</h1></header></div><main></main></body>`,
			expected: "header\n  h1 site\nmain\n",
		},
		{
			name: "landmark roles",
			html: `<body><div role="banner"><h1>site</h1></div><div role="main"><div role="navigation" aria-label="Pages"><a href="/a">A</a></div></div>
				<div role="contentinfo"></div></body>`,
			expected: "header\n  h1 site\nmain\n  nav [Pages]\n    - A (/a)\nfooter\n",
		},
		{
			name:     "explicit roles are landmarks anywhere",
			html:     `<body><main><section><div role="banner"><h2>b</h2></div></section></main></body>`,
			expected: "main\n  section\n    header\n      h2 b\n",
		},
		{
			name: "nav links and their names",
			html: `<body><nav><ul><li><a href="/a"> Plain  text </a></li><li><a href=" /b " aria-label="Labeled">x</a></li>
				<li><a href="/c"><img src="c.png" alt="Logo"></a></li><li><a href="/d" title="Titled"><svg aria-hidden="true"></svg></a></li>
				<li><a>no href</a></li><li hidden><a href="/e">hidden</a></li><li><a href="/f">Sub <a href="/g">nested</a></a></li></ul></nav></body>`,
			expected: "nav\n  - Plain text (/a)\n  - Labeled (/b)\n  - Logo (/c)\n  - Titled (/d)\n  - Sub (/f)\n  - nested (/g)\n",
		},
		{
			name:     "a nav keeps its headings",
			html:     `<body><nav aria-label="Menu"><h2>Shop</h2><a href="/shop">All</a><section><h3>Women</h3><a href="/w">Women</a></section></nav></body>`,
			expected: "nav [Menu]\n  - All (/shop)\n  - Women (/w)\n  h2 Shop\n  section\n    h3 Women\n",
		},
		{
			name:     "hidden subtrees are left out",
			html:     `<body><nav hidden><a href="/a">a</a></nav><main><section aria-hidden="true"><h2>gone</h2></section><h2>kept</h2></main></body>`,
			expected: "main\n  h2 kept\n",
		},
		{
			name:     "only the visible main is the main",
			html:     `<body><main hidden><h1>hidden</h1></main><main><h1>first</h1></main><main><h2>second</h2></main></body>`,
			expected: "main\n  h1 first\nh2 second\n",
		},
		{
			name:     "content next to the main",
			html:     `<body><main><h1>page</h1></main><aside><h2>ads</h2></aside><div><h2>more</h2><p>text</p></div></body>`,
			expected: "main\n  h1 page\naside\n  h2 ads\ndiv\n  h2 more\n",
		},
	})
}

func TestExtractDocument_Roles(t *testing.T) {
	runOutlineTests(t, []outlineTest{
		{
			name:     "role main",
			html:     `<div role="banner"><h1>site</h1></div><div role="main"><h1>page</h1></div>`,
			expected: "h1 page\n",
		},
		{
			name: "container roles",
			html: `<main><div role="article"><h1>a</h1></div><div role="region" aria-label="r"><h2>b</h2></div>
				<span role="complementary"><h2>c</h2></span></main>`,
			expected: "article\n  h1 a\nsection [r]\n  h2 b\naside\n  h2 c\n",
		},
		{
			name: "heading role and levels",
			html: `<main><div role="heading">default</div><span role="heading" aria-level="4">four</span>
				<h2 aria-level="3">native</h2><div role="heading" aria-level="9">deep</div></main>`,
			expected: "h2 default\nh4 four\nh3 native\nh6 deep\n",
		},
		{
			name:     "the first known role counts",
			html:     `<main><div role="foo region"><h2>a</h2></div></main>`,
			expected: "section\n  h2 a\n",
		},
		{
			name:     "none and presentation remove semantics",
			html:     `<main><section role="none"><h2 role="presentation">x</h2><article role="presentation"><h3>a</h3></article></section></main>`,
			expected: "h3 a\n",
		},
		{
			name:     "a presentational div is no container",
			html:     `<main><h1>page</h1><div role="presentation"><h2>a</h2><p>text</p></div></main>`,
			expected: "h1 page\nh2 a\n",
		},
		{
			name:     "other roles keep the element's semantics",
			html:     `<main><article role="listitem"><h2>a</h2></article></main>`,
			expected: "article\n  h2 a\n",
		},
	})
}

func TestExtractDocument_Names(t *testing.T) {
	runOutlineTests(t, []outlineTest{
		{
			name: "container labels",
			html: `<main><section aria-labelledby="t s"><h2 id="t">Title</h2><p id="s">and more</p></section>
				<article aria-label=" Teaser  label "><h2>a</h2></article><aside aria-labelledby="missing" aria-label="fallback"></aside></main>`,
			expected: "section [Title and more]\n  h2 Title\narticle [Teaser label]\n  h2 a\naside [fallback]\n",
		},
		{
			name:     "labelledby reads hidden elements and their aria-label",
			html:     `<main><section aria-labelledby="h l"><span id="h" hidden>hidden</span><span id="l" aria-label="named">text</span><h2>a</h2></section></main>`,
			expected: "section [hidden named]\n  h2 a\n",
		},
		{
			name:     "containers take no name from content",
			html:     `<main><section><h2>a</h2></section></main>`,
			expected: "section\n  h2 a\n",
		},
		{
			name:     "a div container takes the label of its box",
			html:     `<main><h1>page</h1><div class="outer"><div class="inner" aria-label="box"><h2>a</h2>text</div></div></main>`,
			expected: "h1 page\ndiv.inner [box]\n  h2 a\n",
		},
		{
			name: "heading names from content",
			html: `<main><h2>Logo <img src="l.png" alt="Shop"> <svg aria-hidden="true"><text>x</text></svg><span hidden>gone</span></h2>
				<h2><a href="/a" title="Linked"><svg aria-hidden="true"></svg></a></h2>
				<h2><span role="img" aria-label="Star"></span> rated</h2>
				<h2 title="Tooltip"></h2>
				<h2>Decorative <img src="d.png" alt="" title="not read"><img src="u.png" title="Untitled"></h2></main>`,
			expected: "h2 Logo Shop\nh2 Linked\nh2 Star rated\nh2 Tooltip\nh2 Decorative Untitled\n",
		},
		{
			name:     "heading labels win over content",
			html:     `<main><h2 aria-label="Label">content</h2><h2 aria-labelledby="x">content</h2><p id="x">Referenced</p></main>`,
			expected: "h2 Label\nh2 Referenced\n",
		},
	})
}

// classes are the class attributes of the elements' selector targets
func classes(t *testing.T, htmlBytes []byte, elements []DocumentElement) []string {
	t.Helper()
	page, err := goquery.NewDocumentFromReader(bytes.NewReader(htmlBytes))
	require.NoError(t, err)
	var result []string
	for _, e := range elements {
		result = append(result, page.Find(e.Selector).AttrOr("class", ""))
	}
	return result
}

type outlineTest struct {
	name     string
	html     string
	expected string // outline, "-" for no main, a div line may name its class: div.outer
}

// runContentTests compares the outline of the whole body with content
func runContentTests(t *testing.T, tests []outlineTest) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := extractContent(t, []byte(tt.html))
			assert.Equal(t, tt.expected, outline(doc.Body))
			assertSelectors(t, []byte(tt.html), doc.Body)
		})
	}
}

// runBodyTests compares the outline of the whole body
func runBodyTests(t *testing.T, tests []outlineTest) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := extract(t, []byte(tt.html))
			assert.Equal(t, tt.expected, outline(doc.Body))
			assertSelectors(t, []byte(tt.html), doc.Body)
		})
	}
}

func runOutlineTests(t *testing.T, tests []outlineTest) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := extract(t, []byte(tt.html))
			if tt.expected == "-" {
				assert.Nil(t, doc.Main())
				return
			}
			actual := outline(mainOf(t, doc))
			if strings.Contains(tt.expected, "div.") {
				actual = outlineWithClasses(t, []byte(tt.html), mainOf(t, doc))
			}
			assert.Equal(t, tt.expected, actual)
			assertSelectors(t, []byte(tt.html), mainOf(t, doc))
		})
	}
}

// contentHidden removes what Options{Content: true} adds: the content
// elements - an outline element in one stays - and the links of everything
// but navs. The macOS app shows the content per branch from one document with
// content, by hiding it the same way.
func contentHidden(elements []DocumentElement) []DocumentElement {
	content := map[ElementType]bool{
		ElementTypeText: true, ElementTypeImage: true, ElementTypeFigure: true,
		ElementTypeUnorderedList: true, ElementTypeOrderedList: true, ElementTypeListItem: true,
		ElementTypeDefinitions: true, ElementTypeTerm: true, ElementTypeDefinition: true,
		ElementTypeTable: true, ElementTypeRow: true, ElementTypeHeaderCell: true, ElementTypeCell: true,
	}
	result := []DocumentElement{}
	for _, e := range elements {
		if content[e.ElementType] {
			result = append(result, contentHidden(e.Children)...)
			continue
		}
		if e.ElementType != ElementTypeNav {
			e.Links = nil
		}
		e.Children = contentHidden(e.Children)
		result = append(result, e)
	}
	return result
}

func normalized(elements []DocumentElement) []DocumentElement {
	result := []DocumentElement{}
	for _, e := range elements {
		e.Children = normalized(e.Children)
		if len(e.Links) == 0 {
			e.Links = nil
		}
		result = append(result, e)
	}
	return result
}

func TestExtractDocument_ContentKeepsTheOutline(t *testing.T) {
	pages := map[string][]byte{}
	for _, name := range []string{"keeping-sourdough-alive.html", "keeping-sourdough-alive-divs.html", "keeping-sourdough-alive-aria.html", "news-portal-home.html"} {
		pages[name] = readTestData(t, name)
	}
	for name, htmlBytes := range pages {
		t.Run(name, func(t *testing.T) {
			structure := extract(t, htmlBytes)
			withContent := extractContent(t, htmlBytes)
			assert.Equal(t, normalized(structure.Body), normalized(contentHidden(withContent.Body)))
		})
	}
}
