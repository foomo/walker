package htmlschema

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Root is a loaded schema root directory:
//
//	<root>/default.html          fallback for groups without a schema
//	<root>/groups/<group>.html   one schema per group
//	everything else is organized freely and used with val:include and val:comp
//
// A Root is immutable after loading and safe for concurrent use.
type Root struct {
	dir        string
	files      map[string]*rawNode // key: root relative path without extension, e.g. /partials/head
	components map[string]*component
	// componentList is sorted by path, for a deterministic recognition
	componentList []*component
	markers       map[string]*component
	schemas       map[string]*schema
}

// LoadRoot loads and compiles all schemas below dir. Problems in schema files
// are returned as LintErrors.
func LoadRoot(dir string) (*Root, error) {
	return LoadRootOverlay(dir, nil)
}

// LoadRootOverlay loads a schema root like LoadRoot, the contents of overlay
// - absolute file paths, e.g. unsaved editor buffers - take precedence over
// the files on disk
func LoadRootOverlay(dir string, overlay map[string][]byte) (*Root, error) {
	r := &Root{
		dir:        dir,
		files:      map[string]*rawNode{},
		components: map[string]*component{},
		markers:    map[string]*component{},
		schemas:    map[string]*schema{},
	}
	var errs LintErrors
	add := func(p string, data []byte) error {
		rel, errRel := filepath.Rel(dir, p)
		if errRel != nil {
			return errRel
		}
		rel = filepath.ToSlash(rel)
		doc, syntaxErrs := parseSyntax(rel, data)
		errs = append(errs, syntaxErrs...)
		r.files[fileKey(rel)] = doc
		return nil
	}
	seen := map[string]bool{}
	errWalk := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), ".") && p != dir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".html") {
			return nil
		}
		seen[p] = true
		if data, ok := overlay[p]; ok {
			return add(p, data)
		}
		data, errRead := os.ReadFile(p)
		if errRead != nil {
			return errRead
		}
		return add(p, data)
	})
	if errWalk != nil {
		return nil, errWalk
	}
	// files that only exist in the overlay, e.g. new and not saved yet
	for p, data := range overlay {
		rel, errRel := filepath.Rel(dir, p)
		if seen[p] || errRel != nil || strings.HasPrefix(rel, "..") || !strings.HasSuffix(p, ".html") {
			continue
		}
		if err := add(p, data); err != nil {
			return nil, err
		}
	}

	// every file with a val:component root is a recognized component
	labels := map[string]int{}
	for key, doc := range r.files {
		if len(doc.children) == 1 && doc.children[0].name == "val:component" {
			comp := &component{path: key, label: path.Base(key), node: doc.children[0]}
			r.components[key] = comp
			r.componentList = append(r.componentList, comp)
			labels[comp.label]++
		}
	}
	sort.Slice(r.componentList, func(i, j int) bool { return r.componentList[i].path < r.componentList[j].path })
	for _, comp := range r.componentList {
		if labels[comp.label] > 1 {
			comp.label = comp.path
		}
	}

	c := &compiler{root: r}
	for _, comp := range r.componentList {
		c.compileComponent(comp)
		if comp.marker != "" {
			if other, ok := r.markers[comp.marker]; ok {
				c.errorf(comp.node, "marker %q is already used by %s", comp.marker, other.path)
			}
			r.markers[comp.marker] = comp
		}
	}
	for _, comp := range r.componentList {
		c.componentMax(comp)
	}
	keys := make([]string, 0, len(r.files))
	for key := range r.files {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key == "/default" || strings.HasPrefix(key, "/groups/") {
			r.schemas[key] = c.compileSchema(key)
		}
	}
	errs = append(errs, c.errs...)
	if len(errs) > 0 {
		return nil, uniqueErrors(errs)
	}
	return r, nil
}

func (c *compiler) compileSchema(key string) *schema {
	s := &schema{path: key, settings: defaultSettings()}
	c.settings = &s.settings
	c.settingsNode = nil
	c.stack = []string{key}
	s.elements = c.compileNodes(c.root.files[key].children, nil, nil, false)
	for _, e := range s.elements {
		m, _ := c.computeMax(e)
		s.max += m
	}
	return s
}

// schemaFor resolves a group to its schema: groups/<group>.html, then default.html
func (r *Root) schemaFor(group string) (*schema, error) {
	if s, ok := r.schemas["/groups/"+strings.Trim(group, "/")]; ok {
		return s, nil
	}
	if s, ok := r.schemas["/default"]; ok {
		return s, nil
	}
	return nil, errors.New("no schema for group " + strconv.Quote(group) + " and no default.html")
}

// resolvePath resolves a schema path: "/x" is relative to the root, anything
// else to the directory of the file key it is used in
func resolvePath(fromKey string, p string) string {
	p = strings.TrimSuffix(p, ".html")
	if strings.HasPrefix(p, "/") {
		return path.Clean(p)
	}
	return path.Join(path.Dir(fromKey), p)
}

func sourceFile(source string) string {
	i := strings.LastIndex(source, ":")
	if i < 0 {
		return source
	}
	return source[:i]
}

func sourceLine(source string) int {
	line, _ := strconv.Atoi(source[strings.LastIndex(source, ":")+1:])
	return line
}

func uniqueErrors(errs LintErrors) LintErrors {
	seen := map[LintError]bool{}
	unique := LintErrors{}
	for _, e := range errs {
		if !seen[e] {
			seen[e] = true
			unique = append(unique, e)
		}
	}
	sort.SliceStable(unique, func(i, j int) bool {
		if unique[i].File != unique[j].File {
			return unique[i].File < unique[j].File
		}
		return unique[i].Line < unique[j].Line
	})
	return unique
}

// Groups are the names of all group schemas, sorted
func (r *Root) Groups() []string {
	groups := []string{}
	for key := range r.schemas {
		if group, ok := strings.CutPrefix(key, "/groups/"); ok {
			groups = append(groups, group)
		}
	}
	sort.Strings(groups)
	return groups
}

// HasDefault reports whether the root has a default.html for groups without a schema
func (r *Root) HasDefault() bool {
	_, ok := r.schemas["/default"]
	return ok
}

// RootVersion is a fingerprint of all schema files below dir - paths, sizes
// and modification times. It changes when a file is edited, added or removed,
// so tools can reload without parsing anything.
func RootVersion(dir string) (string, error) {
	hash := sha256.New()
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), ".") && p != dir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".html") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(hash, "%s %d %d\n", p, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil))[:16], nil
}
