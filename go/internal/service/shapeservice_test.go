package service

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/foomo/walker/go/internal/config"
	"github.com/foomo/walker/go/internal/demoshop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func demoHandler(t *testing.T) http.Handler {
	t.Helper()
	handler, err := demoshop.NewHandler(demoshop.EmbeddedFiles(), false)
	require.NoError(t, err)
	return handler
}

func TestGetShapeRoot(t *testing.T) {
	s := &service{conf: &config.Config{GroupHeader: demoshop.GroupHeader}}
	info := s.GetShapeRoot(demoshop.ShapesDir())
	assert.Empty(t, info.Error)
	assert.Equal(t, []string{"category", "index", "product"}, info.Groups)
	assert.True(t, info.HasDefault)
	assert.Equal(t, demoshop.GroupHeader, info.GroupHeader)

	broken := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(broken, "default.semanticshape.html"), []byte(`<html><body><h1 score=1/></body></html>`), 0o644))
	info = s.GetShapeRoot(broken)
	assert.Equal(t, "the shape root has errors", info.Error)
	require.Len(t, info.LintErrors, 1)
	assert.Equal(t, "default.semanticshape.html", info.LintErrors[0].File)

	assert.Equal(t, "no shape root", s.GetShapeRoot("").Error)
}

func TestValidateShapeRendered(t *testing.T) {
	w := httptest.NewRecorder()
	demoHandler(t).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://localhost/product/car-04", nil))
	s := &service{}
	validation := s.ValidateShape(demoshop.ShapesDir(), "product", "", "http://localhost/product/car-04", w.Body.String())
	assert.Empty(t, validation.Error)
	assert.Equal(t, ShapeSourceRendered, validation.Source)
	require.NotNil(t, validation.Report)
	assert.Equal(t, 90, validation.Report.Penalty)
	assert.Equal(t, 90, validation.Trace.Penalty)
}

func TestValidateShapeServer(t *testing.T) {
	shop := demoHandler(t)
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-agent", r.UserAgent())
		shop.ServeHTTP(w, r)
	}))
	defer page.Close()
	s := &service{conf: &config.Config{GroupHeader: demoshop.GroupHeader, Agent: "test-agent"}}
	validation := s.ValidateShape(demoshop.ShapesDir(), "", "", page.URL+"/product/robot-01", "")
	assert.Empty(t, validation.Error)
	assert.Equal(t, ShapeSourceServer, validation.Source)
	assert.Equal(t, http.StatusOK, validation.StatusCode)
	assert.Equal(t, "product", validation.Group, "the group comes from the header")
	assert.Equal(t, demoshop.GroupHeader, validation.GroupHeader)
	assert.Equal(t, 121, validation.Report.Score.Points)

	// a header passed in wins over the config's - an edited one, before the walker restarts
	stale := &service{conf: &config.Config{GroupHeader: "x-old-header", Agent: "test-agent"}}
	validation = stale.ValidateShape(demoshop.ShapesDir(), "", demoshop.GroupHeader, page.URL+"/product/robot-01", "")
	assert.Equal(t, "product", validation.Group)
	assert.Equal(t, demoshop.GroupHeader, validation.GroupHeader)
}

func TestValidateShapeServerErrorPage(t *testing.T) {
	page := httptest.NewServer(http.NotFoundHandler())
	defer page.Close()
	validation := (&service{}).ValidateShape(demoshop.ShapesDir(), "product", "", page.URL+"/product/robot-01", "")
	assert.Equal(t, http.StatusNotFound, validation.StatusCode)
	assert.Equal(t, "the page returned 404 Not Found - nothing validated", validation.Error)
	assert.Nil(t, validation.Report)
}
