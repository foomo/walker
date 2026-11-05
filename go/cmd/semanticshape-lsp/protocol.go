package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
	"sync"
)

// the parts of the language server protocol the server needs - json-rpc 2.0
// with Content-Length framing over stdio

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *responseError  `json:"error,omitempty"`
}

type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type conn struct {
	in  *bufio.Reader
	out io.Writer
	mu  sync.Mutex
}

func (c *conn) read() (*message, error) {
	header, err := textproto.NewReader(c.in).ReadMIMEHeader()
	if err != nil {
		return nil, err
	}
	length, err := strconv.Atoi(header.Get("Content-Length"))
	if err != nil {
		return nil, fmt.Errorf("invalid Content-Length: %w", err)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(c.in, body); err != nil {
		return nil, err
	}
	m := &message{}
	return m, json.Unmarshal(body, m)
}

func (c *conn) write(m *message) error {
	m.JSONRPC = "2.0"
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := fmt.Fprintf(c.out, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = c.out.Write(body)
	return err
}

// reply answers a request - a nil result is sent as null
func (c *conn) reply(id json.RawMessage, result any) error {
	if result == nil {
		return c.write(&message{ID: id, Result: json.RawMessage("null")})
	}
	return c.write(&message{ID: id, Result: result})
}

func (c *conn) notify(method string, params any) error {
	data, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return c.write(&message{Method: method, Params: data})
}

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type textRange struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

type textDocumentPositionParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position position `json:"position"`
}

type diagnostic struct {
	Range    textRange `json:"range"`
	Severity int       `json:"severity"`
	Source   string    `json:"source"`
	Message  string    `json:"message"`
}

type markupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type textEdit struct {
	Range   textRange `json:"range"`
	NewText string    `json:"newText"`
}

type completionItem struct {
	Label            string         `json:"label"`
	Kind             int            `json:"kind,omitempty"`
	Detail           string         `json:"detail,omitempty"`
	Documentation    *markupContent `json:"documentation,omitempty"`
	FilterText       string         `json:"filterText,omitempty"`
	InsertTextFormat int            `json:"insertTextFormat,omitempty"`
	TextEdit         *textEdit      `json:"textEdit,omitempty"`
}

const (
	completionKindProperty = 10
	completionKindValue    = 12
	completionKindFile     = 17
	completionKindStruct   = 22
	insertTextSnippet      = 2
	severityError          = 1
)
