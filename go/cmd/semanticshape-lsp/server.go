package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/foomo/walker/go/internal/semanticshape"
)

// server is a language server for semantic shape files. It only works on
// *.shape.html files inside a schema root - a directory with groups/ or
// default.shape.html - and leaves every other html file alone.
type server struct {
	conn *conn
	// docs are the open documents by uri
	docs map[string]string
	// published are the uris with diagnostics by schema root
	published map[string]map[string]bool

	elements     map[string]semanticshape.VocabularyElement
	elementRules []semanticshape.VocabularyAttribute
	valueRules   []semanticshape.VocabularyAttribute
}

func newServer(c *conn) *server {
	elements, elementRules, valueRules := semanticshape.Vocabulary()
	s := &server{
		conn:         c,
		docs:         map[string]string{},
		published:    map[string]map[string]bool{},
		elements:     map[string]semanticshape.VocabularyElement{},
		elementRules: elementRules,
		valueRules:   valueRules,
	}
	for _, e := range elements {
		s.elements[e.Name] = e
	}
	return s
}

// run serves until exit - errors of single requests are answered, not fatal
func (s *server) run() error {
	shutdown := false
	for {
		m, err := s.conn.read()
		if err != nil {
			return err
		}
		switch m.Method {
		case "initialize":
			err = s.conn.reply(m.ID, map[string]any{
				"capabilities": map[string]any{
					"textDocumentSync": map[string]any{"openClose": true, "change": 1, "save": map[string]any{"includeText": false}},
					"completionProvider": map[string]any{
						"triggerCharacters": []string{"<", ":", "\"", "/", "-"},
					},
					"hoverProvider": true,
				},
				"serverInfo": map[string]any{"name": "semanticshape-lsp"},
			})
		case "shutdown":
			shutdown = true
			err = s.conn.reply(m.ID, nil)
		case "exit":
			if !shutdown {
				return errors.New("exit without shutdown")
			}
			return nil
		case "textDocument/didOpen":
			var p struct {
				TextDocument struct {
					URI  string `json:"uri"`
					Text string `json:"text"`
				} `json:"textDocument"`
			}
			if json.Unmarshal(m.Params, &p) == nil {
				s.docs[p.TextDocument.URI] = p.TextDocument.Text
				s.lint(p.TextDocument.URI)
			}
		case "textDocument/didChange":
			var p struct {
				TextDocument struct {
					URI string `json:"uri"`
				} `json:"textDocument"`
				ContentChanges []struct {
					Text string `json:"text"`
				} `json:"contentChanges"`
			}
			if json.Unmarshal(m.Params, &p) == nil && len(p.ContentChanges) > 0 {
				// full sync - the last change is the whole document
				s.docs[p.TextDocument.URI] = p.ContentChanges[len(p.ContentChanges)-1].Text
				s.lint(p.TextDocument.URI)
			}
		case "textDocument/didSave", "textDocument/didClose":
			var p struct {
				TextDocument struct {
					URI string `json:"uri"`
				} `json:"textDocument"`
			}
			if json.Unmarshal(m.Params, &p) == nil {
				if m.Method == "textDocument/didClose" {
					delete(s.docs, p.TextDocument.URI)
				}
				s.lint(p.TextDocument.URI)
			}
		case "textDocument/completion":
			var p textDocumentPositionParams
			if json.Unmarshal(m.Params, &p) == nil {
				err = s.conn.reply(m.ID, s.complete(p.TextDocument.URI, p.Position))
			} else {
				err = s.conn.reply(m.ID, nil)
			}
		case "textDocument/hover":
			var p textDocumentPositionParams
			if json.Unmarshal(m.Params, &p) == nil {
				err = s.conn.reply(m.ID, s.hover(p.TextDocument.URI, p.Position))
			} else {
				err = s.conn.reply(m.ID, nil)
			}
		default:
			if len(m.ID) > 0 && m.Method != "" {
				err = s.conn.write(&message{ID: m.ID, Error: &responseError{Code: -32601, Message: "method not found: " + m.Method}})
			}
		}
		if err != nil {
			return err
		}
	}
}

func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return ""
	}
	return filepath.FromSlash(u.Path)
}

func pathToURI(p string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(p)}).String()
}

// schemaRoot is the closest directory above a shape file with groups/ or
// default.shape.html - "" for any other file
func schemaRoot(file string) string {
	if !strings.HasSuffix(file, semanticshape.FileSuffix) {
		return ""
	}
	for dir := filepath.Dir(file); ; dir = filepath.Dir(dir) {
		if info, err := os.Stat(filepath.Join(dir, "groups")); err == nil && info.IsDir() {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "default"+semanticshape.FileSuffix)); err == nil {
			return dir
		}
		if parent := filepath.Dir(dir); parent == dir {
			return ""
		}
	}
}

// lint loads the schema root of the document with all open documents and
// publishes the lint errors of every file in it
func (s *server) lint(uri string) {
	file := uriToPath(uri)
	root := schemaRoot(file)
	if file == "" || root == "" {
		return
	}
	overlay := map[string][]byte{}
	for docURI, text := range s.docs {
		if p := uriToPath(docURI); p != "" && strings.HasPrefix(p, root+string(filepath.Separator)) {
			overlay[p] = []byte(text)
		}
	}
	byURI := map[string][]diagnostic{}
	_, err := semanticshape.LoadRootOverlay(root, overlay)
	var lintErrors semanticshape.LintErrors
	switch {
	case errors.As(err, &lintErrors):
		for _, e := range lintErrors {
			p := filepath.Join(root, filepath.FromSlash(e.File))
			text, ok := overlay[p]
			if !ok {
				text, _ = os.ReadFile(p)
			}
			fileURI := pathToURI(p)
			byURI[fileURI] = append(byURI[fileURI], diagnostic{
				Range:    lineRange(string(text), e.Line),
				Severity: severityError,
				Source:   "semanticshape",
				Message:  e.Message,
			})
		}
	case err != nil:
		byURI[uri] = append(byURI[uri], diagnostic{Severity: severityError, Source: "semanticshape", Message: err.Error()})
	}
	previous := s.published[root]
	current := map[string]bool{}
	for fileURI, diagnostics := range byURI {
		current[fileURI] = true
		s.publish(fileURI, diagnostics)
	}
	for fileURI := range previous {
		if !current[fileURI] {
			s.publish(fileURI, []diagnostic{})
		}
	}
	s.published[root] = current
}

func (s *server) publish(uri string, diagnostics []diagnostic) {
	_ = s.conn.notify("textDocument/publishDiagnostics", map[string]any{"uri": uri, "diagnostics": diagnostics})
}

func markdown(value string) *markupContent {
	if value == "" {
		return nil
	}
	return &markupContent{Kind: "markdown", Value: value}
}

// complete offers val:* elements, their attributes, the val: rules of html
// elements and attribute values - all only inside schema roots
func (s *server) complete(uri string, pos position) []completionItem {
	file := uriToPath(uri)
	root := schemaRoot(file)
	text, ok := s.docs[uri]
	if root == "" || !ok {
		return []completionItem{}
	}
	off := offset(text, pos)
	ctx := contextAt(text, off)
	replace := textRange{Start: positionOf(text, ctx.prefixStart), End: pos}
	items := []completionItem{}
	switch ctx.kind {
	case contextTagName:
		names := make([]string, 0, len(s.elements))
		for name := range s.elements {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			e := s.elements[name]
			items = append(items, completionItem{
				Label:            name,
				Kind:             completionKindStruct,
				Documentation:    markdown(e.Doc),
				InsertTextFormat: insertTextSnippet,
				TextEdit:         &textEdit{Range: replace, NewText: elementSnippet(e)},
			})
		}
	case contextAttributeName:
		for _, a := range s.attributesOf(ctx.tag) {
			items = append(items, completionItem{
				Label:            a.Name,
				Kind:             completionKindProperty,
				Documentation:    markdown(a.Doc),
				InsertTextFormat: insertTextSnippet,
				TextEdit:         &textEdit{Range: replace, NewText: attributeSnippet(a)},
			})
		}
	case contextAttributeValue:
		items = s.completeValue(ctx, root, file, text, pos)
	}
	return items
}

// attributesOf are the attributes a tag can take: those of a val:* element,
// or the val: rules of an html element
func (s *server) attributesOf(tag string) []semanticshape.VocabularyAttribute {
	if e, ok := s.elements[tag]; ok {
		return e.Attributes
	}
	if strings.HasPrefix(tag, "val:") {
		return nil
	}
	return s.elementRules
}

func elementSnippet(e semanticshape.VocabularyElement) string {
	attrs := ""
	// the first structural attribute is the one an element is written with -
	// settings have only optional ones
	if len(e.Attributes) > 0 && e.Attributes[0].Structural && e.Name != "val:settings" {
		attrs = " " + e.Attributes[0].Name + `="$1"`
	}
	if e.HasChildren {
		return e.Name + attrs + ">\n\t$0\n</" + e.Name + ">"
	}
	return e.Name + attrs + " $0/>"
}

func attributeSnippet(a semanticshape.VocabularyAttribute) string {
	switch {
	case a.Boolean:
		return a.Name
	case strings.HasSuffix(a.Name, "attr-"):
		return a.Name + `${1:content}="$2"`
	case len(a.Values) > 0:
		return a.Name + `="${1|` + strings.Join(a.Values, ",") + `|}"`
	}
	return a.Name + `="$1"`
}

func (s *server) completeValue(ctx completionContext, root, file, text string, pos position) []completionItem {
	items := []completionItem{}
	name := strings.TrimPrefix(ctx.attribute, "val:")
	add := func(label, doc string, kind int, prefixStart int) {
		items = append(items, completionItem{
			Label:         label,
			Kind:          kind,
			Documentation: markdown(doc),
			TextEdit:      &textEdit{Range: textRange{Start: positionOf(text, prefixStart), End: pos}, NewText: label},
		})
	}
	switch {
	case name == "path" && (ctx.tag == "val:include" || ctx.tag == "val:comp"):
		for _, p := range schemaFiles(root, file) {
			add(p, "", completionKindFile, ctx.prefixStart)
		}
	case strings.HasPrefix(name, "attr-") || name == "attr" || name == "value":
		// rule lists: complete the rule after the last ;
		ruleStart := ctx.prefixStart
		if i := strings.LastIndex(ctx.prefix, ";"); i >= 0 {
			ruleStart += i + 1
			for ruleStart < len(text) && text[ruleStart] == ' ' {
				ruleStart++
			}
		}
		for _, r := range s.valueRules {
			add(r.Name, r.Doc, completionKindValue, ruleStart)
		}
	case name == "for" && ctx.tag == "val:wildcard":
		add("div, span", "Skip layout wrappers.", completionKindValue, ctx.prefixStart)
		add("*", "Skip any element.", completionKindValue, ctx.prefixStart)
	default:
		for _, a := range s.attributesOf(ctx.tag) {
			if strings.TrimPrefix(a.Name, "val:") == name {
				for _, v := range a.Values {
					add(v, "", completionKindValue, ctx.prefixStart)
				}
			}
		}
	}
	return items
}

// schemaFiles are the files of a schema root as val:include and val:comp
// paths: root relative with a leading /, without .shape.html - the file itself is left out
func schemaFiles(root, self string) (paths []string) {
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") && p != root {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(p, semanticshape.FileSuffix) || p == self {
			return nil
		}
		rel, errRel := filepath.Rel(root, p)
		if errRel == nil {
			paths = append(paths, "/"+strings.TrimSuffix(filepath.ToSlash(rel), semanticshape.FileSuffix))
		}
		return nil
	})
	return paths
}

// hover explains val:* elements and rule attributes
func (s *server) hover(uri string, pos position) any {
	text, ok := s.docs[uri]
	if !ok || schemaRoot(uriToPath(uri)) == "" {
		return nil
	}
	off := offset(text, pos)
	word, start := wordAt(text, off)
	if word == "" {
		return nil
	}
	result := func(doc string) any {
		return map[string]any{
			"contents": markupContent{Kind: "markdown", Value: doc},
			"range":    textRange{Start: positionOf(text, start), End: positionOf(text, start+len(word))},
		}
	}
	if e, ok := s.elements[word]; ok {
		return result("**" + e.Name + "**\n\n" + e.Doc)
	}
	ctx := contextAt(text, start)
	if ctx.kind == contextNone || ctx.kind == contextTagName {
		return nil
	}
	for _, a := range s.attributesOf(ctx.tag) {
		if a.Name == word || (strings.HasSuffix(a.Name, "attr-") && strings.HasPrefix(word, a.Name)) {
			doc := a.Doc
			if len(a.Values) > 0 {
				doc += "\n\nValues: `" + strings.Join(a.Values, "`, `") + "`"
			}
			return result("**" + word + "**\n\n" + doc)
		}
	}
	return nil
}
