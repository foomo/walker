package semanticshape

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"golang.org/x/net/html"
)

// LintError is a problem in a shape file
type LintError struct {
	File    string
	Line    int
	Message string
}

func (e LintError) Error() string {
	return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Message)
}

// LintErrors are all problems found while loading a shape root
type LintErrors []LintError

func (errs LintErrors) Error() string {
	lines := make([]string, len(errs))
	for i, e := range errs {
		lines[i] = e.Error()
	}
	return strings.Join(lines, "\n")
}

// rawNode is an element of a shape file, exactly as written
type rawNode struct {
	name     string
	attrs    []html.Attribute
	children []*rawNode
	file     string
	line     int
}

func (n *rawNode) attr(key string) (string, bool) {
	for _, a := range n.attrs {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

func (n *rawNode) source() string {
	return fmt.Sprintf("%s:%d", n.file, n.line)
}

var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true, "img": true,
	"input": true, "link": true, "meta": true, "param": true, "source": true, "track": true, "wbr": true,
}

// parseSyntax reads a shape file with the html tokenizer, but without the
// error recovery of the html5 parser: the tree is exactly as written, "/>"
// closes any element and every other element has to be closed explicitly
func parseSyntax(file string, data []byte) (doc *rawNode, errs LintErrors) {
	doc = &rawNode{file: file, line: 1}
	stack := []*rawNode{doc}
	z := html.NewTokenizer(bytes.NewReader(data))
	line := 1
	for {
		tt := z.Next()
		raw := z.Raw()
		tokenLine := line
		line += bytes.Count(raw, []byte("\n"))
		if tt == html.ErrorToken {
			if z.Err() != io.EOF {
				errs = append(errs, LintError{file, tokenLine, z.Err().Error()})
			}
			break
		}
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			// shapes never contain raw text like a <title> or <script> body
			z.NextIsNotRawText()
			name, hasAttr := z.TagName()
			n := &rawNode{name: string(name), file: file, line: tokenLine}
			for hasAttr {
				var key, val []byte
				key, val, hasAttr = z.TagAttr()
				n.attrs = append(n.attrs, html.Attribute{Key: string(key), Val: string(val)})
			}
			selfClosing := tt == html.SelfClosingTagToken
			// the tokenizer reads <x a=1/> as a="1/" - in a shape that is always meant to close the element
			if !selfClosing && bytes.HasSuffix(raw, []byte("/>")) && len(n.attrs) > 0 && strings.HasSuffix(n.attrs[len(n.attrs)-1].Val, "/") {
				last := &n.attrs[len(n.attrs)-1]
				last.Val = strings.TrimSuffix(last.Val, "/")
				selfClosing = true
			}
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, n)
			if !selfClosing && !voidElements[n.name] {
				stack = append(stack, n)
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			endName := string(name)
			if voidElements[endName] {
				errs = append(errs, LintError{file, tokenLine, "</" + endName + "> closes a void element"})
				continue
			}
			open := -1
			for i := len(stack) - 1; i > 0; i-- {
				if stack[i].name == endName {
					open = i
					break
				}
			}
			if open == -1 {
				errs = append(errs, LintError{file, tokenLine, "</" + endName + "> without an open <" + endName + ">"})
				continue
			}
			for i := len(stack) - 1; i > open; i-- {
				errs = append(errs, LintError{file, stack[i].line, "<" + stack[i].name + "> is not closed"})
			}
			stack = stack[:open]
		}
	}
	for i := len(stack) - 1; i > 0; i-- {
		errs = append(errs, LintError{file, stack[i].line, "<" + stack[i].name + "> is not closed"})
	}
	return doc, errs
}
