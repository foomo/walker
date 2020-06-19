package config

import (
	"testing"

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
address: ":3001"
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
