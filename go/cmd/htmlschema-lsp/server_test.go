package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextAt(t *testing.T) {
	for name, tc := range map[string]struct {
		text string
		want completionContext
	}{
		"tag name":         {`<val:se`, completionContext{kind: contextTagName, prefix: "val:se", prefixStart: 1}},
		"attribute name":   {`<h1 val:sc`, completionContext{kind: contextAttributeName, tag: "h1", prefix: "val:sc", prefixStart: 4}},
		"after a value":    {`<meta name="x" `, completionContext{kind: contextAttributeName, tag: "meta", prefix: "", prefixStart: 15}},
		"attribute value":  {`<val:comp path="/par`, completionContext{kind: contextAttributeValue, tag: "val:comp", attribute: "path", prefix: "/par", prefixStart: 16}},
		"> in a value":     {`<a val:attr-href="regex:^a>b`, completionContext{kind: contextAttributeValue, tag: "a", attribute: "val:attr-href", prefix: "regex:^a>b", prefixStart: 18}},
		"outside of tags":  {`<h1/> text`, completionContext{}},
		"closing tag":      {`</val:cho`, completionContext{}},
		"inside a comment": {`<!-- <h1 val:`, completionContext{}},
		"after a comment":  {`<!-- x --><h1 val:`, completionContext{kind: contextAttributeName, tag: "h1", prefix: "val:", prefixStart: 14}},
		"unquoted value":   {`<h1 val:score=`, completionContext{}},
		"multi line":       {"<title\n    val:min-length=\"30\"\n    val:", completionContext{kind: contextAttributeName, tag: "title", prefix: "val:", prefixStart: 35}},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, contextAt(tc.text, len(tc.text)))
		})
	}
}

func TestPositions(t *testing.T) {
	text := "a\n–b😀c\nd"
	// – is one utf-16 unit, 😀 two
	assert.Equal(t, position{1, 4}, positionOf(text, strings.Index(text, "c")))
	assert.Equal(t, strings.Index(text, "c"), offset(text, position{1, 4}))
	assert.Equal(t, len(text), offset(text, position{5, 0}))
}

// client drives the server over pipes like an editor - it reads in the
// background, pipes do not buffer and the server writes notifications any time
type client struct {
	t        *testing.T
	c        *conn
	id       int
	messages chan *message
}

func startServer(t *testing.T) *client {
	serverIn, clientOut := io.Pipe()
	clientIn, serverOut := io.Pipe()
	s := newServer(&conn{in: bufio.NewReader(serverIn), out: serverOut})
	go func() {
		_ = s.run()
		serverOut.Close()
	}()
	c := &client{t: t, c: &conn{in: bufio.NewReader(clientIn), out: clientOut}, messages: make(chan *message, 100)}
	go func() {
		defer close(c.messages)
		for {
			m, err := c.c.read()
			if err != nil {
				return
			}
			c.messages <- m
		}
	}()
	t.Cleanup(func() { clientOut.Close() })
	return c
}

func (c *client) notify(method string, params any) {
	require.NoError(c.t, c.c.notify(method, params))
}

// request sends a request and returns its result, notifications in between are collected
func (c *client) request(method string, params any, notifications *[]*message) json.RawMessage {
	c.id++
	data, _ := json.Marshal(params)
	id, _ := json.Marshal(c.id)
	require.NoError(c.t, c.c.write(&message{ID: id, Method: method, Params: data}))
	for {
		var m *message
		select {
		case m = <-c.messages:
		case <-time.After(5 * time.Second):
			c.t.Fatal("no response to " + method)
		}
		require.NotNil(c.t, m, "server closed")
		if m.Method != "" {
			if notifications != nil {
				*notifications = append(*notifications, m)
			}
			continue
		}
		result, _ := json.Marshal(m.Result)
		return result
	}
}

func TestServer(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "groups"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "partials"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "partials", "head.html"), []byte(`<title/>`), 0o644))
	file := filepath.Join(root, "groups", "page.html")
	require.NoError(t, os.WriteFile(file, []byte(""), 0o644))
	uri := pathToURI(file)

	c := startServer(t)
	c.request("initialize", map[string]any{}, nil)
	c.notify("initialized", map[string]any{})

	// a forgotten prefix in an unsaved buffer
	text := "<html>\n  <head>\n    <title score=10/>\n    <val:include path=\"\"/>\n  </head>\n</html>\n"
	c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": uri, "text": text}})
	var notifications []*message
	c.request("shutdown-probe", nil, &notifications)
	require.NotEmpty(t, notifications)
	var diagnostics struct {
		URI         string       `json:"uri"`
		Diagnostics []diagnostic `json:"diagnostics"`
	}
	require.NoError(t, json.Unmarshal(notifications[0].Params, &diagnostics))
	assert.Equal(t, uri, diagnostics.URI)
	require.NotEmpty(t, diagnostics.Diagnostics)
	assert.Contains(t, diagnostics.Diagnostics[0].Message, `did you mean "val:score"?`)
	assert.Equal(t, textRange{Start: position{2, 4}, End: position{2, 21}}, diagnostics.Diagnostics[0].Range)

	// completion of include paths and of rules
	pathPos := positionOf(text, strings.Index(text, `path=""`)+len(`path="`))
	var items []completionItem
	require.NoError(t, json.Unmarshal(c.request("textDocument/completion", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": pathPos}, nil), &items))
	assert.Equal(t, []string{"/partials/head"}, labels(items))

	rulePos := positionOf(text, strings.Index(text, "score"))
	require.NoError(t, json.Unmarshal(c.request("textDocument/completion", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": rulePos}, nil), &items))
	assert.Contains(t, labels(items), "val:score")
	assert.Contains(t, labels(items), "val:attr-")

	// hover on an element
	hoverPos := positionOf(text, strings.Index(text, "val:include")+2)
	var hover struct {
		Contents markupContent `json:"contents"`
	}
	require.NoError(t, json.Unmarshal(c.request("textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": hoverPos}, nil), &hover))
	assert.Contains(t, hover.Contents.Value, "**val:include**")

	// fixing the error clears the diagnostics
	fixed := strings.Replace(text, "score=10", "val:score=10", 1)
	fixed = strings.Replace(fixed, `path=""`, `path="/partials/head"`, 1)
	c.notify("textDocument/didChange", map[string]any{"textDocument": map[string]any{"uri": uri}, "contentChanges": []map[string]any{{"text": fixed}}})
	notifications = nil
	c.request("shutdown-probe", nil, &notifications)
	require.NotEmpty(t, notifications)
	require.NoError(t, json.Unmarshal(notifications[0].Params, &diagnostics))
	assert.Empty(t, diagnostics.Diagnostics)
}

func TestServerIgnoresFilesOutsideSchemaRoots(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "page.html")
	c := startServer(t)
	c.request("initialize", map[string]any{}, nil)
	c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": pathToURI(file), "text": "<h1 score=1/>"}})
	var notifications []*message
	var items []completionItem
	result := c.request("textDocument/completion", map[string]any{"textDocument": map[string]any{"uri": pathToURI(file)}, "position": position{0, 4}}, &notifications)
	require.NoError(t, json.Unmarshal(result, &items))
	assert.Empty(t, items)
	assert.Empty(t, notifications)
}

func labels(items []completionItem) (labels []string) {
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	return labels
}
