package main

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// offset converts an lsp position - utf-16 code units in a line - to a byte offset
func offset(text string, pos position) int {
	line := 0
	i := 0
	for line < pos.Line {
		next := strings.IndexByte(text[i:], '\n')
		if next < 0 {
			return len(text)
		}
		i += next + 1
		line++
	}
	units := 0
	for i < len(text) && text[i] != '\n' && units < pos.Character {
		r, size := utf8.DecodeRuneInString(text[i:])
		units += len(utf16.Encode([]rune{r}))
		i += size
	}
	return i
}

// positionOf converts a byte offset to an lsp position
func positionOf(text string, off int) position {
	if off > len(text) {
		off = len(text)
	}
	pos := position{}
	lineStart := 0
	for i := 0; i < off; i++ {
		if text[i] == '\n' {
			pos.Line++
			lineStart = i + 1
		}
	}
	for _, r := range text[lineStart:off] {
		pos.Character += len(utf16.Encode([]rune{r}))
	}
	return pos
}

// lineRange is the range of a whole line, 1 based as in LintError
func lineRange(text string, line int) textRange {
	lines := strings.Split(text, "\n")
	if line < 1 || line > len(lines) {
		return textRange{}
	}
	content := lines[line-1]
	start := len(content) - len(strings.TrimLeft(content, " \t"))
	end := len(strings.TrimRight(content, " \t\r"))
	return textRange{
		Start: position{Line: line - 1, Character: len(utf16.Encode([]rune(content[:start])))},
		End:   position{Line: line - 1, Character: len(utf16.Encode([]rune(content[:end])))},
	}
}

type contextKind int

const (
	contextNone contextKind = iota
	contextTagName
	contextAttributeName
	contextAttributeValue
)

// completionContext is where the cursor is in a tag
type completionContext struct {
	kind contextKind
	tag  string
	// attribute whose value is edited
	attribute string
	// prefix is the partial word before the cursor, replaced by a completion
	prefix      string
	prefixStart int
}

func isNameByte(b byte) bool {
	return b == ':' || b == '-' || b == '_' || b == '.' || b == '/' ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// contextAt finds the tag the cursor is in - quotes are tracked, so a > in an
// attribute value does not end the tag
func contextAt(text string, off int) completionContext {
	start := tagStart(text, off)
	if start < 0 {
		return completionContext{}
	}
	inner := text[start+1 : off]
	if strings.HasPrefix(inner, "/") || strings.HasPrefix(inner, "!") {
		return completionContext{}
	}
	nameEnd := strings.IndexAny(inner, " \t\r\n")
	if nameEnd < 0 {
		return completionContext{kind: contextTagName, prefix: inner, prefixStart: start + 1}
	}
	ctx := completionContext{tag: inner[:nameEnd]}
	// walk the attributes, tracking quotes
	var quote byte
	valueStart := 0
	key := ""
	i := nameEnd
	for i < len(inner) {
		c := inner[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
				key = ""
			}
		case c == '"' || c == '\'':
			quote = c
			valueStart = i + 1
		case c == '=':
			j := i - 1
			for j >= 0 && (inner[j] == ' ' || inner[j] == '\t') {
				j--
			}
			k := j
			for k >= 0 && isNameByte(inner[k]) {
				k--
			}
			key = inner[k+1 : j+1]
		}
		i++
	}
	if quote != 0 {
		ctx.kind = contextAttributeValue
		ctx.attribute = key
		ctx.prefix = inner[valueStart:]
		ctx.prefixStart = start + 1 + valueStart
		return ctx
	}
	j := len(inner)
	for j > 0 && isNameByte(inner[j-1]) {
		j--
	}
	if j > 0 && inner[j-1] == '=' {
		// after = without a quote - nothing to complete
		return completionContext{}
	}
	ctx.kind = contextAttributeName
	ctx.prefix = inner[j:]
	ctx.prefixStart = start + 1 + j
	return ctx
}

// tagStart scans the text up to off and returns the start of the tag the
// cursor is in, -1 outside of tags - comments and quoted values are skipped
func tagStart(text string, off int) int {
	start := -1
	var quote byte
	for i := 0; i < off; i++ {
		c := text[i]
		switch {
		case start < 0 && strings.HasPrefix(text[i:], "<!--"):
			end := strings.Index(text[i+4:], "-->")
			if end < 0 || i+4+end+3 > off {
				return -1
			}
			i += 4 + end + 2
		case start < 0:
			if c == '<' {
				start = i
			}
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			start = -1
		case c == '<':
			// a < in a tag starts a new one - the old one was broken
			start = i
		}
	}
	return start
}

// wordAt is the name under the cursor, for hover
func wordAt(text string, off int) (word string, start int) {
	s := off
	for s > 0 && isNameByte(text[s-1]) {
		s--
	}
	e := off
	for e < len(text) && isNameByte(text[e]) {
		e++
	}
	return text[s:e], s
}
