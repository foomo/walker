package htmlschema

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/pkg/testutil/assert"
)

func TestGroupValidator(t *testing.T) {
	gv, errGV := NewGroupValidator(filepath.Join(getSchemaDir(), "groups"))
	if errGV != nil {
		t.Error(errGV)
	}
	schemaCatalogueProduct := gv.getSchemaForGroup("catalogue/product")
	assert.NotNil(t, schemaCatalogueProduct)
	report, errValidate := gv.Validate("catalogue/product", []byte(`<html></html>`), os.Stdout)
	assert.NilError(t, errValidate)
	assert.NotNil(t, report)
	report.Print(os.Stdout)
}
