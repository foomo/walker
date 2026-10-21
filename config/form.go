package config

import (
	"bytes"
	"strconv"
	"strings"

	yaml "gopkg.in/yaml.v3"
)

// Form is a config as an editor shows it - every key with its value or its
// default. Durations stay strings, as written.
type Form struct {
	BaseURL           string   `json:"baseURL"`
	Paths             []string `json:"paths"`
	Concurrency       int      `json:"concurrency"`
	Addr              string   `json:"addr"`
	Agent             string   `json:"agent"`
	UseCookies        bool     `json:"useCookies"`
	Ignore            []string `json:"ignore"`
	IgnoreQueriesWith []string `json:"ignoreQueriesWith"`
	IgnoreAllQueries  bool     `json:"ignoreAllQueries"`
	IgnoreRobots      bool     `json:"ignoreRobots"`
	Depth             int      `json:"depth"`
	Paging            bool     `json:"paging"`
	GroupHeader       string   `json:"groupHeader"`
	SchemaRoot        string   `json:"schemaRoot"`
	Interval          string   `json:"interval"`
	Offset            string   `json:"offset"`
}

// DefaultForm has the defaults of Load
func DefaultForm() Form {
	return Form{
		Concurrency:       2,
		Addr:              ":3001",
		Agent:             "foomo-walker",
		UseCookies:        true,
		Paths:             []string{},
		Ignore:            []string{},
		IgnoreQueriesWith: []string{},
	}
}

// ReadForm reads a config into a form - leniently, a value of the wrong
// type keeps its default, Validate reports it
func ReadForm(data []byte) (Form, error) {
	form := DefaultForm()
	root, err := mappingRoot(data)
	if err != nil || root == nil {
		return form, err
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, n := root.Content[i].Value, root.Content[i+1]
		switch key {
		case "target":
			readTarget(&form, n)
		case "concurrency":
			readInt(n, &form.Concurrency)
		case "depth":
			readInt(n, &form.Depth)
		case "addr":
			form.Addr = scalar(n, form.Addr)
		case "agent":
			form.Agent = scalar(n, form.Agent)
		case "groupheader":
			form.GroupHeader = scalar(n, form.GroupHeader)
		case "schemaroot":
			form.SchemaRoot = scalar(n, form.SchemaRoot)
		case "interval":
			form.Interval = scalar(n, form.Interval)
		case "offset":
			form.Offset = scalar(n, form.Offset)
		case "usecookies":
			readBool(n, &form.UseCookies)
		case "ignoreallqueries":
			readBool(n, &form.IgnoreAllQueries)
		case "ignorerobots":
			readBool(n, &form.IgnoreRobots)
		case "paging":
			readBool(n, &form.Paging)
		case "ignore":
			form.Ignore = stringList(n)
		case "ignorequerieswith":
			form.IgnoreQueriesWith = stringList(n)
		}
	}
	return form, nil
}

// WriteForm writes a form into a config. The existing yaml is edited in place:
// comments, key order and keys the form does not know survive. New keys are
// appended, keys that are absent and at their default stay absent.
func WriteForm(data []byte, form Form) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		root.Kind, root.Tag, root.Value, root.Content = yaml.MappingNode, "", "", nil
	}
	defaults := DefaultForm()
	w := &formWriter{root: root}

	w.target(form)
	w.int("concurrency", form.Concurrency, defaults.Concurrency)
	w.str("addr", form.Addr, defaults.Addr)
	w.str("agent", form.Agent, defaults.Agent)
	w.bool("usecookies", form.UseCookies, defaults.UseCookies)
	w.strings("ignore", form.Ignore)
	w.strings("ignorequerieswith", form.IgnoreQueriesWith)
	w.bool("ignoreallqueries", form.IgnoreAllQueries, defaults.IgnoreAllQueries)
	w.bool("ignorerobots", form.IgnoreRobots, defaults.IgnoreRobots)
	w.int("depth", form.Depth, defaults.Depth)
	w.bool("paging", form.Paging, defaults.Paging)
	w.str("groupheader", form.GroupHeader, "")
	w.str("schemaroot", form.SchemaRoot, "")
	w.str("interval", form.Interval, "")
	w.str("offset", form.Offset, "")

	buf := &bytes.Buffer{}
	encoder := yaml.NewEncoder(buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&doc); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type formWriter struct {
	root *yaml.Node
}

// value is the value node of key, nil if the key is not there
func (w *formWriter) value(key string) *yaml.Node {
	for i := 0; i+1 < len(w.root.Content); i += 2 {
		if w.root.Content[i].Value == key {
			return w.root.Content[i+1]
		}
	}
	return nil
}

func (w *formWriter) remove(key string) {
	for i := 0; i+1 < len(w.root.Content); i += 2 {
		if w.root.Content[i].Value == key {
			w.root.Content = append(w.root.Content[:i], w.root.Content[i+2:]...)
			return
		}
	}
}

// set replaces the value of key in place - its comments stay - or appends it
func (w *formWriter) set(key string, value *yaml.Node) {
	for i := 0; i+1 < len(w.root.Content); i += 2 {
		k, n := w.root.Content[i], w.root.Content[i+1]
		if k.Value != key {
			continue
		}
		head, line, foot := n.HeadComment, n.LineComment, n.FootComment
		changesKind := n.Kind != value.Kind
		*n = *value
		n.HeadComment, n.FootComment = head, foot
		if changesKind && value.Kind != yaml.ScalarNode {
			// a list or mapping would write the comment at its end - it belongs to the key line
			if k.LineComment == "" {
				k.LineComment = line
			}
		} else {
			n.LineComment = line
		}
		return
	}
	w.root.Content = append(w.root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
}

func (w *formWriter) str(key, value, defaultValue string) {
	switch {
	case w.value(key) == nil && value == defaultValue:
	case value == "" && defaultValue == "":
		w.remove(key)
	default:
		w.set(key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
	}
}

func (w *formWriter) int(key string, value, defaultValue int) {
	if w.value(key) == nil && value == defaultValue {
		return
	}
	w.set(key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(value)})
}

func (w *formWriter) bool(key string, value, defaultValue bool) {
	if w.value(key) == nil && value == defaultValue {
		return
	}
	w.set(key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(value)})
}

func (w *formWriter) strings(key string, values []string) {
	if len(values) == 0 {
		w.remove(key)
		return
	}
	w.set(key, sequence(values))
}

// target is a plain url, or a mapping when there are paths
func (w *formWriter) target(form Form) {
	if len(form.Paths) == 0 {
		w.set("target", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: form.BaseURL})
		return
	}
	w.set("target", &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "baseurl"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: form.BaseURL},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "paths"},
		sequence(form.Paths),
	}})
}

func sequence(values []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode}
	for _, v := range values {
		n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v})
	}
	return n
}

func mappingRoot(data []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil
	}
	return doc.Content[0], nil
}

func readTarget(form *Form, n *yaml.Node) {
	switch n.Kind {
	case yaml.ScalarNode:
		form.BaseURL = n.Value
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			switch strings.ToLower(n.Content[i].Value) {
			case "baseurl":
				form.BaseURL = scalar(n.Content[i+1], form.BaseURL)
			case "paths":
				form.Paths = stringList(n.Content[i+1])
			}
		}
	}
}

func scalar(n *yaml.Node, fallback string) string {
	if n.Kind != yaml.ScalarNode {
		return fallback
	}
	return n.Value
}

func readInt(n *yaml.Node, target *int) {
	if v, err := strconv.Atoi(n.Value); n.Kind == yaml.ScalarNode && err == nil {
		*target = v
	}
}

func readBool(n *yaml.Node, target *bool) {
	var b bool
	if n.Kind == yaml.ScalarNode && n.Decode(&b) == nil {
		*target = b
	}
}

func stringList(n *yaml.Node) []string {
	list := []string{}
	if n.Kind != yaml.SequenceNode {
		return list
	}
	for _, item := range n.Content {
		if item.Kind == yaml.ScalarNode {
			list = append(list, item.Value)
		}
	}
	return list
}
