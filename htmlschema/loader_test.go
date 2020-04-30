package htmlschema

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func getExampleDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "example")
}

func getSchemaDir() string {
	root := getExampleDir()
	return filepath.Join(root, "schema")
}

func TestLoad(t *testing.T) {
	root := getExampleDir()
	fileCMSIndexSchema := filepath.Join(root, "schema", "groups", "content", "index.html")
	schema, errLoad := Load(fileCMSIndexSchema)
	if errLoad != nil {
		panic(errLoad)
	}
	fileCMSIndex := filepath.Join(root, "htdocs", "content", "page-a.html")
	indexBytes, _ := ioutil.ReadFile(fileCMSIndex)
	report, errValidate := schema.Validate(indexBytes, os.Stdout)
	if errValidate != nil {
		panic(errValidate)
	}
	report.Print(os.Stdout)
}
