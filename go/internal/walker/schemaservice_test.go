package walker

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/foomo/walker/go/internal/config"
	"github.com/foomo/walker/go/internal/htmlschema/demo/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func demoSchemaRoot() string {
	return getExampleDir("..", "htmlschema", "demo", "schema")
}

func demoHandler(t *testing.T) http.Handler {
	t.Helper()
	handler, err := server.NewHandler(server.EmbeddedFiles(), false)
	require.NoError(t, err)
	return handler
}

func TestGetSchemaRoot(t *testing.T) {
	s := &service{conf: &config.Config{GroupHeader: server.GroupHeader}}
	info := s.GetSchemaRoot(demoSchemaRoot())
	assert.Empty(t, info.Error)
	assert.Equal(t, []string{"category", "index", "product"}, info.Groups)
	assert.True(t, info.HasDefault)
	assert.Equal(t, server.GroupHeader, info.GroupHeader)

	broken := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(broken, "default.html"), []byte(`<html><body><h1 score=1/></body></html>`), 0o644))
	info = s.GetSchemaRoot(broken)
	assert.Equal(t, "the schema root has errors", info.Error)
	require.Len(t, info.LintErrors, 1)
	assert.Equal(t, "default.html", info.LintErrors[0].File)

	assert.Equal(t, "no schema root", s.GetSchemaRoot("").Error)
}

func TestValidateSchemaRendered(t *testing.T) {
	w := httptest.NewRecorder()
	demoHandler(t).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://localhost/product/car-04", nil))
	s := &service{}
	validation := s.ValidateSchema(demoSchemaRoot(), "product", "http://localhost/product/car-04", w.Body.String())
	assert.Empty(t, validation.Error)
	assert.Equal(t, SchemaSourceRendered, validation.Source)
	require.NotNil(t, validation.Report)
	assert.Equal(t, 90, validation.Report.Penalty)
	assert.Equal(t, 90, validation.Trace.Penalty)
}

func TestValidateSchemaServer(t *testing.T) {
	shop := demoHandler(t)
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-agent", r.UserAgent())
		shop.ServeHTTP(w, r)
	}))
	defer page.Close()
	s := &service{conf: &config.Config{GroupHeader: server.GroupHeader, Agent: "test-agent"}}
	validation := s.ValidateSchema(demoSchemaRoot(), "", page.URL+"/product/robot-01", "")
	assert.Empty(t, validation.Error)
	assert.Equal(t, SchemaSourceServer, validation.Source)
	assert.Equal(t, http.StatusOK, validation.StatusCode)
	assert.Equal(t, "product", validation.Group, "the group comes from the header")
	assert.Equal(t, 121, validation.Report.Score.Points)
}

func TestValidateSchemaServerErrorPage(t *testing.T) {
	page := httptest.NewServer(http.NotFoundHandler())
	defer page.Close()
	validation := (&service{}).ValidateSchema(demoSchemaRoot(), "product", page.URL+"/product/robot-01", "")
	assert.Equal(t, http.StatusNotFound, validation.StatusCode)
	assert.Equal(t, "the page returned 404 Not Found - nothing validated", validation.Error)
	assert.Nil(t, validation.Report)
}
