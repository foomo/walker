package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	confComplexTarget = `
---
target:
  baseurl: https://www.bestbytes.de
  paths:
    - /projects
    - /
concurrency: 1
ignoreallqueries: true
depth: 4
paging: false
agent: foomo-walker
addr: ":3001"
schemaroot: htmlschema/example/schema/bestbytes
...
`
	confComplexMinimal = `
---
target: https://www.bestbytes.de
...
`
	confComplexMinimalPath = `
---
target: https://www.bestbytes.de/foo
...
`
)

func TestLoad(t *testing.T) {
	cnf, errCnf := Load([]byte(confComplexTarget))
	assert.NoError(t, errCnf)
	assert.Equal(t, "https://www.bestbytes.de", cnf.Target.BaseURL)

	cnf, errCnf = Load([]byte(confComplexMinimal))
	assert.NoError(t, errCnf)
	assert.Equal(t, "https://www.bestbytes.de", cnf.Target.BaseURL)
	assert.Equal(t, []string{"/"}, cnf.Target.Paths)

	cnf, errCnf = Load([]byte(confComplexMinimalPath))
	assert.NoError(t, errCnf)
	assert.Equal(t, "https://www.bestbytes.de", cnf.Target.BaseURL)
	assert.Equal(t, []string{"/foo"}, cnf.Target.Paths)
}

func TestLoadInterval(t *testing.T) {
	cnf, errCnf := Load([]byte("target: https://www.bestbytes.de\ninterval: 24h\noffset: 3h30m\n"))
	assert.NoError(t, errCnf)
	assert.Equal(t, 24*time.Hour, cnf.Interval)
	assert.Equal(t, 3*time.Hour+30*time.Minute, cnf.Offset)

	cnf, errCnf = Load([]byte(confComplexMinimal))
	assert.NoError(t, errCnf)
	assert.Equal(t, time.Duration(0), cnf.Interval)

	for _, invalid := range []string{
		"interval: -1h",
		"interval: 1h\noffset: 1h",
		"interval: 1h\noffset: -1m",
		"offset: 1h",
		"interval: nonsense",
	} {
		_, errCnf = Load([]byte("target: https://www.bestbytes.de\n" + invalid + "\n"))
		assert.Error(t, errCnf, invalid)
	}
}
