package semanticshape

import (
	"sort"
	"strings"
)

// VocabularyAttribute describes an attribute of the shape language for
// editor tooling - completion and hover
type VocabularyAttribute struct {
	// Name as written: unprefixed on val:* elements, with val: on html elements and val:comp
	Name    string
	Doc     string
	Values  []string
	Boolean bool
	// Structural attributes say what an element is, like for, path or type - they are not rules
	Structural bool
}

// VocabularyElement describes a val:* element
type VocabularyElement struct {
	Name       string
	Doc        string
	Attributes []VocabularyAttribute
	// HasChildren is true for elements that contain other shape elements
	HasChildren bool
}

var ruleDocs = map[string]string{
	"min":         "Minimum number of matches. Without max: no upper limit.",
	"max":         "Maximum number of matches, `*` for unbounded. Without min: at least 1 (0 inside val:choice).",
	"count":       "Exact number of matches.",
	"optional":    "May be missing: 0..1, combines with max.",
	"forbidden":   "Must not occur: 0..0.",
	"min-length":  "Minimum length of the normalized text in characters, inclusive.",
	"max-length":  "Maximum length of the normalized text in characters, inclusive.",
	"min-words":   "Minimum number of words of the text, inclusive.",
	"max-words":   "Maximum number of words of the text, inclusive.",
	"score":       "Points every matched node earns if its own rules pass.",
	"score-per":   "Scale the points with the amount of content: `words:k` or `chars:k` - score × floor(units / k).",
	"score-max":   "Caps everything the element earns, subtree included, summed over all its nodes.",
	"severity":    "Severity of the findings of this element's own rules: error (default), warn or info. On val:comp a ceiling for the whole instance.",
	"penalty":     "Penalty of the findings of this element - default by severity from val:settings (error 10, warn 3, info 0).",
	"penalty-max": "Caps the total occurrence penalty of this element.",
	"open":        "Component context: components that are not used here are only info instead of an error.",
	"attr":        "v1 form of an attribute rule: `name; rule; rule` - prefer attr-<name>.",
	"attr-*":      "Rules for the value of an attribute: `rule; rule; …` e.g. `min-length:70; max-length:160`.",
	"value":       "Rules for the JSON-LD value: `rule; rule; …` - the same rules as attr-*.",
	"every":       "Every array element on the path has to pass - and has the property, if it is required. Without it one passing element is enough.",
}

var ruleValues = map[string][]string{
	"severity":  {"error", "warn", "info"},
	"score-per": {"words:", "chars:"},
	"max":       {"*"},
}

var booleanRules = map[string]bool{"optional": true, "forbidden": true, "open": true, "every": true}

var structuralDocs = map[string]map[string]string{
	"val:selector":  {"for": "CSS selector, searched in the whole subtree - ignores wildcards and component borders."},
	"val:wildcard":  {"for": "CSS selector list of the wrappers to skip, e.g. `div, span`. `*` skips anything, empty makes the scope strict."},
	"val:include":   {"path": "File to insert in place: `/` = shape root, otherwise relative, without .semanticshape.html."},
	"val:comp":      {"path": "File with a single root element to instantiate: `/` = shape root, otherwise relative, without .semanticshape.html."},
	"val:component": {"marker": "Value of the marker attribute (data-component) of the instances.", "selector": "CSS selector for instances without a marker."},
	"val:ld":        {"type": "JSON-LD @type of the objects, also inside @graph."},
	"val:prop":      {"path": "Dot path into the JSON-LD object, arrays fan out: `offers.price`."},
	"val:settings": {
		"exclude":          "CSS selectors of elements to ignore, e.g. responsive duplicates `.mobile-only`.",
		"marker-attribute": "Attribute that marks component instances, default data-component.",
		"penalty-error":    "Default penalty of error findings (10).",
		"penalty-warn":     "Default penalty of warn findings (3).",
		"penalty-info":     "Default penalty of info findings (0).",
	},
}

var elementDocs = map[string]string{
	"val:selector":  "Matches all elements a CSS selector finds in the subtree, page wide at the top of body. The rules of the val:selector apply to every selected node. Default 1..unbounded.",
	"val:wildcard":  "Scope in which the wrappers matching `for` between the described elements are skipped - lexical, inherited by everything inside.",
	"val:include":   "Inserts a file in place, as if it was written here.",
	"val:comp":      "Instantiates the single root element of a file - its attributes override the root's. A val:component root is a recognized component.",
	"val:component": "Root of a component file: instances are recognized by marker, or by selector, and are opaque for the shape around them.",
	"val:choice":    "Any number of the listed components in any order - min/max limit the total.",
	"val:sequence":  "The listed components in this order.",
	"val:ld":        "JSON-LD objects of a @type, page wide - or inside the instance in a component.",
	"val:prop":      "A property of the JSON-LD object - required unless optional.",
	"val:settings":  "Settings of the shape, a direct child of html - shareable with val:include.",
}

var elementKinds = map[string]kind{
	"val:selector":  kindSelector,
	"val:choice":    kindChoice,
	"val:sequence":  kindSequence,
	"val:ld":        kindLD,
	"val:prop":      kindProp,
	"val:component": kindComp,
}

var elementHasChildren = map[string]bool{
	"val:selector": true, "val:wildcard": true, "val:component": true, "val:choice": true, "val:sequence": true, "val:ld": true,
}

func ruleAttribute(name, prefix string) VocabularyAttribute {
	doc := ruleDocs[name]
	if strings.HasPrefix(name, "attr-") {
		doc = ruleDocs["attr-*"]
	}
	return VocabularyAttribute{Name: prefix + name, Doc: doc, Values: ruleValues[name], Boolean: booleanRules[name]}
}

func ruleNames(k kind) (names []string) {
	for name := range allowedRules[k] {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func rulesOf(k kind, prefix string) (attributes []VocabularyAttribute) {
	for _, name := range ruleNames(k) {
		if name == "attr-*" {
			name = "attr-"
		}
		attributes = append(attributes, ruleAttribute(name, prefix))
	}
	return attributes
}

// Vocabulary describes the shape language: the val:* elements, the rules
// of html elements (with val: prefix) and the rules of a rule list
func Vocabulary() (elements []VocabularyElement, elementRules []VocabularyAttribute, valueRules []VocabularyAttribute) {
	names := make([]string, 0, len(elementDocs))
	for name := range elementDocs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		e := VocabularyElement{Name: name, Doc: elementDocs[name], HasChildren: elementHasChildren[name]}
		for _, s := range structuralAttrs[name] {
			e.Attributes = append(e.Attributes, VocabularyAttribute{Name: s, Doc: structuralDocs[name][s], Structural: true})
		}
		switch name {
		case "val:comp":
			// val:comp passes its attributes through to the root, rules keep the prefix
			e.Attributes = append(e.Attributes, rulesOf(kindComp, "val:")...)
		default:
			if k, ok := elementKinds[name]; ok {
				e.Attributes = append(e.Attributes, rulesOf(k, "")...)
			}
		}
		elements = append(elements, e)
	}
	elementRules = rulesOf(kindElement, "val:")
	valueRules = []VocabularyAttribute{
		{Name: "required", Doc: "Present and not empty."},
		{Name: "empty", Doc: "Absent or empty."},
		{Name: "min-length:", Doc: "Minimum length in characters, inclusive."},
		{Name: "max-length:", Doc: "Maximum length in characters, inclusive."},
		{Name: "length:", Doc: "Exact length in characters."},
		{Name: "min-words:", Doc: "Minimum number of words."},
		{Name: "max-words:", Doc: "Maximum number of words."},
		{Name: "one-of:", Doc: "One of the alternatives: `one-of:a|b|c`."},
		{Name: "url", Doc: "Parses as a URL."},
		{Name: "url:absolute", Doc: "An absolute http(s) URL."},
		{Name: "regex:", Doc: "Go regexp - must be the last rule, takes everything after `regex:`."},
		{Name: "severity:", Doc: "Severity of this attribute's findings: error, warn or info."},
		{Name: "penalty:", Doc: "Penalty of this attribute's findings."},
	}
	return elements, elementRules, valueRules
}
