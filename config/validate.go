package config

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/foomo/walker/htmlschema"
	yaml "gopkg.in/yaml.v3"
)

const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

// Problem is something wrong or doubtful in a config file
type Problem struct {
	Severity string `json:"severity"`
	// Key is the config key the problem is about, "" for the whole file
	Key     string `json:"key"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
}

func (p Problem) String() string {
	return fmt.Sprintf("%d:%d %s %s", p.Line, p.Column, p.Severity, p.Message)
}

type keyType int

const (
	typeString keyType = iota
	typeInt
	typeBool
	typeStrings
	typeDuration
	typeTarget
)

// keys are the config keys - yaml.v3 matches the lower case field names exactly
var keys = map[string]keyType{
	"concurrency":       typeInt,
	"addr":              typeString,
	"target":            typeTarget,
	"ignore":            typeStrings,
	"ignorequerieswith": typeStrings,
	"ignoreallqueries":  typeBool,
	"usecookies":        typeBool,
	"depth":             typeInt,
	"paging":            typeBool,
	"ignorerobots":      typeBool,
	"groupheader":       typeString,
	"agent":             typeString,
	"schemaroot":        typeString,
	"interval":          typeDuration,
	"offset":            typeDuration,
}

// Validate checks a config file: unknown keys, types, values and the
// schema root - relative paths are resolved against base, the directory of
// the config file. Every problem has the line it is about.
func Validate(data []byte, base string) []Problem {
	v := &validator{}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return []Problem{yamlProblem(err)}
	}
	if len(doc.Content) == 0 {
		return []Problem{{Severity: SeverityError, Key: "target", Line: 1, Column: 1, Message: "the config is empty - target is required"}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return []Problem{{Severity: SeverityError, Line: root.Line, Column: root.Column, Message: "the config has to be a mapping of keys"}}
	}
	values := map[string]*yaml.Node{}
	for i := 0; i+1 < len(root.Content); i += 2 {
		k, value := root.Content[i], root.Content[i+1]
		key := k.Value
		if first, ok := values[key]; ok {
			v.add(SeverityError, key, k, fmt.Sprintf("%s is set twice, first in line %d", key, first.Line))
			continue
		}
		values[key] = value
		t, known := keys[key]
		if !known {
			v.unknown(key, k)
			continue
		}
		v.check(key, t, value)
	}
	target, ok := values["target"]
	if !ok {
		v.problems = append(v.problems, Problem{Severity: SeverityError, Key: "target", Line: root.Line, Column: root.Column, Message: "target is required - the url to crawl"})
	} else {
		v.target(target)
	}
	v.schema(values, base)
	// the validator and walker must never disagree
	if _, err := Load(data); err != nil && !v.hasErrors() {
		v.problems = append(v.problems, Problem{Severity: SeverityError, Line: root.Line, Column: root.Column, Message: err.Error()})
	}
	sort.SliceStable(v.problems, func(i, j int) bool { return v.problems[i].Line < v.problems[j].Line })
	return v.problems
}

type validator struct {
	problems []Problem
}

func (v *validator) add(severity, key string, n *yaml.Node, message string) {
	v.problems = append(v.problems, Problem{Severity: severity, Key: key, Line: n.Line, Column: n.Column, Message: message})
}

func (v *validator) hasErrors() bool {
	for _, p := range v.problems {
		if p.Severity == SeverityError {
			return true
		}
	}
	return false
}

// unknown reports a key walker ignores - with a suggestion if one is close
func (v *validator) unknown(key string, n *yaml.Node) {
	lower := strings.ToLower(key)
	if _, ok := keys[lower]; ok && lower != key {
		v.add(SeverityWarning, key, n, fmt.Sprintf("unknown key %q is ignored - keys are lower case: %q", key, lower))
		return
	}
	best, distance := "", 4
	for known := range keys {
		if d := editDistance(lower, known); d < distance || (d == distance && known < best) {
			best, distance = known, d
		}
	}
	if best != "" {
		v.add(SeverityWarning, key, n, fmt.Sprintf("unknown key %q is ignored - did you mean %q?", key, best))
		return
	}
	v.add(SeverityWarning, key, n, fmt.Sprintf("unknown key %q is ignored", key))
}

func (v *validator) check(key string, t keyType, n *yaml.Node) {
	switch t {
	case typeString:
		if n.Kind != yaml.ScalarNode {
			v.add(SeverityError, key, n, key+" has to be a string")
			return
		}
		if key == "addr" {
			if _, _, err := net.SplitHostPort(n.Value); err != nil {
				v.add(SeverityError, key, n, fmt.Sprintf("addr %q has to be host:port, e.g. \":3001\"", n.Value))
			}
		}
	case typeInt:
		value, err := strconv.Atoi(n.Value)
		if n.Kind != yaml.ScalarNode || err != nil {
			v.add(SeverityError, key, n, key+" has to be a number")
			return
		}
		if key == "concurrency" && value < 1 {
			v.add(SeverityError, key, n, "concurrency has to be at least 1")
		}
		if key == "concurrency" && value > 20 {
			v.add(SeverityWarning, key, n, fmt.Sprintf("concurrency %d can take a site down", value))
		}
		if key == "depth" && value < 0 {
			v.add(SeverityError, key, n, "depth must not be negative")
		}
	case typeBool:
		var b bool
		if n.Kind != yaml.ScalarNode || n.Decode(&b) != nil {
			v.add(SeverityError, key, n, key+" has to be true or false")
		}
	case typeStrings:
		if n.Kind != yaml.SequenceNode {
			v.add(SeverityError, key, n, key+" has to be a list")
			return
		}
		for _, item := range n.Content {
			if item.Kind != yaml.ScalarNode {
				v.add(SeverityError, key, item, key+" entries have to be strings")
				continue
			}
			if key == "ignore" && !strings.HasPrefix(item.Value, "/") {
				v.add(SeverityWarning, key, item, fmt.Sprintf("ignore %q is a path prefix and should start with /", item.Value))
			}
		}
	case typeDuration:
		if n.Kind != yaml.ScalarNode {
			v.add(SeverityError, key, n, key+" has to be a duration like 1h or 30m")
			return
		}
		// yaml can not read a plain number into a duration - walker would not start
		if _, err := strconv.ParseInt(n.Value, 10, 64); err == nil && n.Value != "0" {
			v.add(SeverityError, key, n, fmt.Sprintf("%s %s is a plain number - write a duration like 1h or 30m", key, n.Value))
			return
		}
		if d, err := time.ParseDuration(n.Value); err != nil && n.Value != "0" {
			v.add(SeverityError, key, n, fmt.Sprintf("%s %q is no duration - e.g. 1h, 30m, 90s", key, n.Value))
		} else if d < 0 {
			v.add(SeverityError, key, n, key+" must not be negative")
		}
	}
}

func (v *validator) target(n *yaml.Node) {
	switch n.Kind {
	case yaml.ScalarNode:
		v.url("target", n)
	case yaml.MappingNode:
		hasBaseURL := false
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, value := n.Content[i], n.Content[i+1]
			switch strings.ToLower(k.Value) {
			case "baseurl":
				hasBaseURL = true
				if value.Kind != yaml.ScalarNode {
					v.add(SeverityError, "target", value, "target.baseurl has to be a url")
					continue
				}
				v.url("target", value)
			case "paths":
				if value.Kind != yaml.SequenceNode {
					v.add(SeverityError, "target", value, "target.paths has to be a list of paths")
					continue
				}
				for _, p := range value.Content {
					if !strings.HasPrefix(p.Value, "/") {
						v.add(SeverityWarning, "target", p, fmt.Sprintf("path %q should start with /", p.Value))
					}
				}
			default:
				v.add(SeverityWarning, "target", k, fmt.Sprintf("unknown key target.%s is ignored - target takes baseurl and paths", k.Value))
			}
		}
		if !hasBaseURL {
			v.add(SeverityError, "target", n, "target needs a baseurl")
		}
	default:
		v.add(SeverityError, "target", n, "target has to be a url or a mapping with baseurl and paths")
	}
}

func (v *validator) url(key string, n *yaml.Node) {
	u, err := url.Parse(n.Value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		v.add(SeverityError, key, n, fmt.Sprintf("%q is not an absolute http(s) url", n.Value))
	}
}

// schema checks the schema root and its interplay with the group header
func (v *validator) schema(values map[string]*yaml.Node, base string) {
	root, hasRoot := values["schemaroot"]
	header, hasHeader := values["groupheader"]
	if hasHeader && !hasRoot && header.Value != "" {
		v.add(SeverityInfo, "groupheader", header, "the group header is only used together with a schemaroot")
	}
	if !hasRoot || root.Kind != yaml.ScalarNode || root.Value == "" {
		return
	}
	if !hasHeader || header.Value == "" {
		v.add(SeverityInfo, "schemaroot", root, "without a groupheader every page is validated with default.html")
	}
	dir := root.Value
	if !filepath.IsAbs(dir) && base != "" {
		dir = filepath.Join(base, dir)
	}
	info, err := os.Stat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		v.add(SeverityError, "schemaroot", root, fmt.Sprintf("schemaroot %s does not exist", dir))
		return
	case err != nil:
		// e.g. a sandboxed helper of the desktop app
		v.add(SeverityInfo, "schemaroot", root, fmt.Sprintf("schemaroot %s could not be checked: %s", dir, err))
		return
	case !info.IsDir():
		v.add(SeverityError, "schemaroot", root, fmt.Sprintf("schemaroot %s is not a directory", dir))
		return
	}
	_, errLoad := htmlschema.LoadRoot(dir)
	var lintErrors htmlschema.LintErrors
	switch {
	case errors.As(errLoad, &lintErrors):
		for _, e := range lintErrors {
			v.add(SeverityError, "schemaroot", root, fmt.Sprintf("schema %s:%d: %s", e.File, e.Line, e.Message))
		}
	case errLoad != nil:
		v.add(SeverityInfo, "schemaroot", root, fmt.Sprintf("schemaroot %s could not be loaded: %s", dir, errLoad))
	}
}

var yamlLine = regexp.MustCompile(`line (\d+)`)

// yamlProblem turns a yaml syntax error into a problem with its line
func yamlProblem(err error) Problem {
	p := Problem{Severity: SeverityError, Line: 1, Column: 1, Message: err.Error()}
	if m := yamlLine.FindStringSubmatch(err.Error()); m != nil {
		p.Line, _ = strconv.Atoi(m[1])
	}
	return p
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
