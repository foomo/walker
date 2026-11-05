package htmlschema

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Severity of a finding - the gate that decides whether a page passes
type Severity string

const (
	SeverityError Severity = "error"
	SeverityWarn  Severity = "warn"
	SeverityInfo  Severity = "info"
)

func (s Severity) rank() int {
	switch s {
	case SeverityError:
		return 3
	case SeverityWarn:
		return 2
	case SeverityInfo:
		return 1
	}
	return 0
}

func parseSeverity(s string) (Severity, error) {
	switch sev := Severity(s); sev {
	case SeverityError, SeverityWarn, SeverityInfo:
		return sev, nil
	}
	return "", fmt.Errorf("unknown severity %q, expected error, warn or info", s)
}

// capSeverity lowers s to the ceiling, if there is one
func capSeverity(s, ceiling Severity) Severity {
	if ceiling != "" && s.rank() > ceiling.rank() {
		return ceiling
	}
	return s
}

const unbounded = -1

type occurrence struct {
	min int
	max int // unbounded or >= 0
}

func (o occurrence) String() string {
	switch {
	case o.min == o.max:
		return "exactly " + strconv.Itoa(o.min)
	case o.max == unbounded:
		return "at least " + strconv.Itoa(o.min)
	case o.min == 0:
		return "at most " + strconv.Itoa(o.max)
	}
	return strconv.Itoa(o.min) + ".." + strconv.Itoa(o.max)
}

// textRules apply to the normalized text content of a node, -1 means unset
type textRules struct {
	minLength int
	maxLength int
	minWords  int
	maxWords  int
}

func newTextRules() textRules {
	return textRules{minLength: -1, maxLength: -1, minWords: -1, maxWords: -1}
}

func (t textRules) empty() bool {
	return t.minLength < 0 && t.maxLength < 0 && t.minWords < 0 && t.maxWords < 0
}

type textFailure struct {
	rule    string
	message string
}

func (t textRules) check(text string) (failures []textFailure) {
	length := utf8.RuneCountInString(text)
	words := len(strings.Fields(text))
	if t.minLength >= 0 && length < t.minLength {
		failures = append(failures, textFailure{"min-length", fmt.Sprintf("got %d, expected at least %d", length, t.minLength)})
	}
	if t.maxLength >= 0 && length > t.maxLength {
		failures = append(failures, textFailure{"max-length", fmt.Sprintf("got %d, expected at most %d", length, t.maxLength)})
	}
	if t.minWords >= 0 && words < t.minWords {
		failures = append(failures, textFailure{"min-words", fmt.Sprintf("got %d, expected at least %d", words, t.minWords)})
	}
	if t.maxWords >= 0 && words > t.maxWords {
		failures = append(failures, textFailure{"max-words", fmt.Sprintf("got %d, expected at most %d", words, t.maxWords)})
	}
	return failures
}

func normalizeText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// valueCheck is one rule of a rule list like "min-length:5; max-length:80"
type valueCheck struct {
	name  string
	check func(val string) (ok bool, message string)
}

// valueRules is a parsed rule list, used for attributes and JSON-LD values
type valueRules struct {
	checks   []valueCheck
	severity Severity // "" = the element's severity
	penalty  int      // -1 = the element's penalty
}

// attrRule validates the value of one attribute
type attrRule struct {
	attr string
	valueRules
}

func parseValueRules(spec string) (rules valueRules, err error) {
	rules.penalty = -1
	rest := spec
	for strings.TrimSpace(rest) != "" {
		part := rest
		rest = ""
		trimmed := strings.TrimSpace(part)
		// regex has to be the last rule, it takes everything after "regex:"
		if !strings.HasPrefix(trimmed, "regex:") {
			if i := strings.Index(part, ";"); i >= 0 {
				part, rest = part[:i], part[i+1:]
			}
		}
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, arg, hasArg := strings.Cut(part, ":")
		name = strings.TrimSpace(name)
		if name != "regex" {
			arg = strings.TrimSpace(arg)
		}
		intArg := func() (int, error) {
			n, errAtoi := strconv.Atoi(arg)
			if errAtoi != nil || n < 0 {
				return 0, fmt.Errorf("rule %q needs a number, got %q", name, arg)
			}
			return n, nil
		}
		switch name {
		case "required":
			rules.checks = append(rules.checks, valueCheck{name, func(val string) (bool, string) {
				return val != "", "missing or empty"
			}})
		case "empty":
			rules.checks = append(rules.checks, valueCheck{name, func(val string) (bool, string) {
				return val == "", fmt.Sprintf("got %q, expected empty", val)
			}})
		case "min-length", "max-length", "length", "min-words", "max-words":
			n, errN := intArg()
			if errN != nil {
				return rules, errN
			}
			rules.checks = append(rules.checks, valueCheck{name, measureCheck(name, n)})
		case "one-of":
			if !hasArg || arg == "" {
				return rules, errors.New(`rule "one-of" needs alternatives, e.g. one-of:a|b`)
			}
			alternatives := strings.Split(arg, "|")
			rules.checks = append(rules.checks, valueCheck{name, func(val string) (bool, string) {
				for _, a := range alternatives {
					if val == a {
						return true, ""
					}
				}
				return false, fmt.Sprintf("%q is not one of %s", val, arg)
			}})
		case "url":
			switch arg {
			case "":
				rules.checks = append(rules.checks, valueCheck{name, func(val string) (bool, string) {
					_, errParse := url.Parse(val)
					return val != "" && errParse == nil, fmt.Sprintf("%q is not a URL", val)
				}})
			case "absolute":
				rules.checks = append(rules.checks, valueCheck{name, func(val string) (bool, string) {
					u, errParse := url.Parse(val)
					return errParse == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "", fmt.Sprintf("%q is not an absolute URL", val)
				}})
			default:
				return rules, fmt.Errorf("unknown url rule %q, expected url or url:absolute", arg)
			}
		case "regex":
			re, errCompile := regexp.Compile(strings.TrimSpace(arg))
			if errCompile != nil {
				return rules, fmt.Errorf("invalid regex: %w", errCompile)
			}
			rules.checks = append(rules.checks, valueCheck{name, func(val string) (bool, string) {
				return re.MatchString(val), fmt.Sprintf("%q does not match %s", val, re)
			}})
		case "severity":
			sev, errSev := parseSeverity(arg)
			if errSev != nil {
				return rules, errSev
			}
			rules.severity = sev
		case "penalty":
			n, errN := intArg()
			if errN != nil {
				return rules, errN
			}
			rules.penalty = n
		default:
			return rules, fmt.Errorf("unknown rule %q", name)
		}
	}
	return rules, nil
}

func measureCheck(name string, n int) func(val string) (bool, string) {
	return func(val string) (bool, string) {
		got := utf8.RuneCountInString(val)
		if strings.HasSuffix(name, "words") {
			got = len(strings.Fields(val))
		}
		switch name {
		case "min-length", "min-words":
			return got >= n, fmt.Sprintf("got %d, expected at least %d", got, n)
		case "max-length", "max-words":
			return got <= n, fmt.Sprintf("got %d, expected at most %d", got, n)
		default:
			return got == n, fmt.Sprintf("got %d, expected exactly %d", got, n)
		}
	}
}

// failures returns the checks val does not pass
func (r valueRules) failures(val string) (failed []valueCheck, messages []string) {
	for _, c := range r.checks {
		if ok, message := c.check(val); !ok {
			failed = append(failed, c)
			messages = append(messages, message)
		}
	}
	return failed, messages
}
