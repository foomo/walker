package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type problem struct {
	severity string
	key      string
	line     int
}

func problems(list []Problem) (result []problem) {
	for _, p := range list {
		result = append(result, problem{p.Severity, p.Key, p.Line})
	}
	return result
}

func TestValidate(t *testing.T) {
	for name, tc := range map[string]struct {
		yaml string
		want []problem
		text string
	}{
		"minimal": {"target: https://www.bestbytes.de\n", nil, ""},
		// the fixture of config_test.go points to a schema root that is gone
		"complex":          {confComplexTarget, []problem{{SeverityInfo, "schemaroot", 14}, {SeverityError, "schemaroot", 14}}, "without a groupheader"},
		"typo address":     {"target: https://example.com\nagent: Yandex\naddress: \":3001\"\n", []problem{{SeverityWarning, "address", 3}}, `did you mean "addr"?`},
		"upper case key":   {"Target: https://example.com\ntarget: https://example.com\n", []problem{{SeverityWarning, "Target", 1}}, `keys are lower case: "target"`},
		"missing target":   {"concurrency: 2\n", []problem{{SeverityError, "target", 1}}, "target is required"},
		"relative target":  {"target: /foo\n", []problem{{SeverityError, "target", 1}}, "not an absolute http(s) url"},
		"concurrency zero": {"target: https://example.com\nconcurrency: 0\n", []problem{{SeverityError, "concurrency", 2}}, "at least 1"},
		"not a number":     {"target: https://example.com\ndepth: deep\n", []problem{{SeverityError, "depth", 2}}, "has to be a number"},
		"bad duration":     {"target: https://example.com\ninterval: 1 hour\n", []problem{{SeverityError, "interval", 2}}, "is no duration"},
		"plain number":     {"target: https://example.com\ninterval: 3600\n", []problem{{SeverityError, "interval", 2}}, "plain number"},
		"offset too big":   {"target: https://example.com\ninterval: 1h\noffset: 2h\n", []problem{{SeverityError, "", 1}}, "offset must be"},
		"bad addr":         {"target: https://example.com\naddr: 3001\n", []problem{{SeverityError, "addr", 2}}, "host:port"},
		"ignore prefix":    {"target: https://example.com\nignore:\n  - /ok\n  - nope\n", []problem{{SeverityWarning, "ignore", 4}}, "should start with /"},
		"duplicate key":    {"target: https://example.com\ndepth: 1\ndepth: 2\n", []problem{{SeverityError, "depth", 3}}, "set twice, first in line 2"},
		"target mapping":   {"target:\n  baseurl: https://example.com\n  path:\n    - /\n", []problem{{SeverityWarning, "target", 3}}, "target.path is ignored"},
		"yaml syntax":      {"target: https://example.com\n  depth: [\n", []problem{{SeverityError, "", 2}}, ""},
		"header only":      {"target: https://example.com\ngroupheader: x-group\n", []problem{{SeverityInfo, "groupheader", 2}}, "only used together with a schemaroot"},
	} {
		t.Run(name, func(t *testing.T) {
			list := Validate([]byte(tc.yaml), "")
			assert.Equal(t, tc.want, problems(list))
			if tc.text != "" && len(list) > 0 {
				assert.Contains(t, list[0].Message, tc.text)
			}
		})
	}
}

func TestValidateSchemaRoot(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, "schema"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "schema", "default.html"), []byte("<html><body><h1 score=1/></body></html>"), 0o644))

	// relative to the config file, with the lint errors of the schema
	list := Validate([]byte("target: https://example.com\ngroupheader: x-group\nschemaroot: schema\n"), base)
	require.Len(t, list, 1)
	assert.Equal(t, problem{SeverityError, "schemaroot", 3}, problems(list)[0])
	assert.Contains(t, list[0].Message, `schema default.html:1: attribute "score" looks like a rule`)

	list = Validate([]byte("target: https://example.com\nschemaroot: nope\n"), base)
	assert.Equal(t, []problem{{SeverityInfo, "schemaroot", 2}, {SeverityError, "schemaroot", 2}}, problems(list))
}

func TestFormRoundTrip(t *testing.T) {
	yaml := `# crawl the shop
target: https://example.com # production
concurrency: 1
unknownkey: stays
interval: 1h
`
	form, err := ReadForm([]byte(yaml))
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", form.BaseURL)
	assert.Equal(t, 1, form.Concurrency)
	assert.Equal(t, ":3001", form.Addr, "defaults for keys that are not there")
	assert.True(t, form.UseCookies)
	assert.Equal(t, "1h", form.Interval)

	form.Concurrency = 4
	form.Paths = []string{"/shop", "/blog"}
	form.Ignore = []string{"/admin"}
	form.Interval = ""
	written, err := WriteForm([]byte(yaml), form)
	require.NoError(t, err)
	out := string(written)
	assert.Contains(t, out, "# crawl the shop")
	assert.Contains(t, out, "unknownkey: stays")
	assert.Contains(t, out, "concurrency: 4")
	assert.Contains(t, out, "ignore:\n  - /admin")
	assert.NotContains(t, out, "interval", "an emptied optional key is removed")
	assert.NotContains(t, out, "addr", "defaults that were not there stay out")

	// it reads back the same, and walker loads it
	again, err := ReadForm(written)
	require.NoError(t, err)
	assert.Equal(t, form, again)
	conf, err := Load(written)
	require.NoError(t, err)
	assert.Equal(t, []string{"/shop", "/blog"}, conf.Target.Paths)
}

func TestFormTargetComment(t *testing.T) {
	form, _ := ReadForm([]byte("target: https://example.com # prod\nfoo: bar\n"))
	form.Paths = []string{"/shop"}
	written, err := WriteForm([]byte("target: https://example.com # prod\nfoo: bar\n"), form)
	require.NoError(t, err)
	assert.Equal(t, "target: # prod\n  baseurl: https://example.com\n  paths:\n    - /shop\nfoo: bar\n", string(written))
}

func TestFormNewFile(t *testing.T) {
	form := DefaultForm()
	form.BaseURL = "https://example.com"
	written, err := WriteForm(nil, form)
	require.NoError(t, err)
	assert.Equal(t, "target: https://example.com\n", string(written))
	assert.Empty(t, Validate(written, ""))
}
