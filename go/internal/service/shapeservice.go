package service

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/foomo/walker/go/internal/semanticshape"
	"github.com/foomo/walker/go/internal/vo"
)

// the shape developer api - client and server share a file system, shape
// roots are paths on the server

const (
	ShapeSourceServer   = "server"
	ShapeSourceRendered = "rendered"
)

// GetShapeRoot loads a shape root and lists its groups
func (s *service) GetShapeRoot(shapeRoot string) (info vo.ShapeRootInfo) {
	start := time.Now()
	defer func() {
		logLintErrors(shapeRoot, info.LintErrors)
		attrs := []any{
			"root", shapeRoot,
			"groups", len(info.Groups),
			"default", info.HasDefault,
			"lintErrors", len(info.LintErrors),
			"duration", time.Since(start),
		}
		if info.Error != "" {
			slog.Warn("shape root failed", append(attrs, "error", info.Error)...)
			return
		}
		slog.Info("shape root loaded", attrs...)
	}()
	info.GroupHeader = s.groupHeader()
	root, lintErrors, err := loadShapeRoot(shapeRoot)
	info.LintErrors = lintErrors
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.Groups = root.Groups()
	info.HasDefault = root.HasDefault()
	return info
}

// GetShapeRootVersion is a fingerprint of the files of a shape root - it
// changes with every edit, the shape developer polls it to reload. Empty if
// the root can not be read.
func (s *service) GetShapeRootVersion(shapeRoot string) string {
	if shapeRoot == "" {
		return ""
	}
	version, err := semanticshape.RootVersion(shapeRoot)
	if err != nil {
		return ""
	}
	return version
}

// ValidateShape validates a page against a shape root, which is loaded on
// every call to pick up changes. Without html the server fetches targetURL,
// otherwise html is a snapshot of the page in a browser. An empty group is
// the group the page reports in the group header, or default.semanticshape.html.
// groupHeader names that response header, empty is the one of the walker's
// config - an edited header is used before the walker restarts.
func (s *service) ValidateShape(shapeRoot, group, groupHeader, targetURL, html string) (validation vo.ShapeValidation) {
	var (
		start                       = time.Now()
		loadDuration, fetchDuration time.Duration
		validateDuration            time.Duration
		requestedGroup, htmlLength  = group, 0
	)
	defer func() {
		logLintErrors(shapeRoot, validation.LintErrors)
		attrs := []any{
			"root", shapeRoot,
			"url", targetURL,
			"source", validation.Source,
			"requestedGroup", requestedGroup,
			"groupHeader", validation.GroupHeader,
			"group", validation.Group,
			"bytes", htmlLength,
			"load", loadDuration,
			"validate", validateDuration,
			"duration", time.Since(start),
		}
		if validation.Source == ShapeSourceServer {
			attrs = append(attrs, "status", validation.StatusCode, "detectedGroup", validation.DetectedGroup, "fetch", fetchDuration)
		}
		if validation.Error != "" {
			slog.Warn("shape validation failed", append(attrs, "lintErrors", len(validation.LintErrors), "error", validation.Error)...)
			return
		}
		r := validation.Report
		attrs = append(attrs,
			"shape", r.Shape,
			"points", r.Score.Points,
			"max", r.Score.Max,
			"penalty", r.Penalty,
			"errors", r.Count(semanticshape.SeverityError),
			"warnings", r.Count(semanticshape.SeverityWarn),
			"infos", r.Count(semanticshape.SeverityInfo),
		)
		slog.Info("shape validated", attrs...)
	}()
	if groupHeader == "" {
		groupHeader = s.groupHeader()
	}
	validation.URL = targetURL
	validation.GroupHeader = groupHeader
	validation.Source = ShapeSourceRendered
	loadStart := time.Now()
	root, lintErrors, err := loadShapeRoot(shapeRoot)
	loadDuration = time.Since(loadStart)
	validation.LintErrors = lintErrors
	if err != nil {
		validation.Error = err.Error()
		return validation
	}
	htmlBytes := []byte(html)
	if html == "" {
		validation.Source = ShapeSourceServer
		fetchStart := time.Now()
		body, statusCode, detectedGroup, errFetch := s.fetch(targetURL, groupHeader)
		fetchDuration = time.Since(fetchStart)
		validation.StatusCode = statusCode
		validation.DetectedGroup = detectedGroup
		if errFetch != nil {
			validation.Error = errFetch.Error()
			return validation
		}
		// an error page would be validated as if it was the page
		if statusCode < 200 || statusCode > 299 {
			validation.Error = fmt.Sprintf("the page returned %d %s - nothing validated", statusCode, http.StatusText(statusCode))
			return validation
		}
		htmlBytes = body
	}
	if group == "" {
		group = validation.DetectedGroup
	}
	validation.Group = group
	htmlLength = len(htmlBytes)
	// the head and the JSON-LD are shown even if the validation fails
	validation.Page, _ = semanticshape.ExtractPageData(htmlBytes)
	validateStart := time.Now()
	report, trace, err := root.Trace(group, htmlBytes)
	validateDuration = time.Since(validateStart)
	if err != nil {
		validation.Error = err.Error()
		return validation
	}
	validation.Report = report
	validation.Trace = trace
	return validation
}

// logLintErrors logs every problem of a shape root on debug level, the
// summaries only count them
func logLintErrors(shapeRoot string, lintErrors []semanticshape.LintError) {
	for _, e := range lintErrors {
		slog.Debug("shape lint error", "root", shapeRoot, "file", e.File, "line", e.Line, "message", e.Message)
	}
}

func loadShapeRoot(shapeRoot string) (*semanticshape.Root, []semanticshape.LintError, error) {
	if shapeRoot == "" {
		return nil, nil, errors.New("no shape root")
	}
	root, err := semanticshape.LoadRoot(shapeRoot)
	var lintErrors semanticshape.LintErrors
	if errors.As(err, &lintErrors) {
		return nil, lintErrors, errors.New("the shape root has errors")
	}
	return root, nil, err
}

func (s *service) groupHeader() string {
	if s.conf == nil {
		return ""
	}
	return s.conf.GroupHeader
}

// fetch loads a page like the crawler does - group is the value of the
// response header groupHeader
func (s *service) fetch(targetURL, groupHeader string) (body []byte, statusCode int, group string, err error) {
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, 0, "", err
	}
	if s.conf != nil && s.conf.Agent != "" {
		req.Header.Set("User-Agent", s.conf.Agent)
	}
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, "", err
	}
	defer resp.Body.Close()
	if groupHeader != "" {
		group = resp.Header.Get(groupHeader)
		if strings.HasSuffix(group, "/") {
			group += "index"
		}
	}
	body, err = io.ReadAll(resp.Body)
	return body, resp.StatusCode, group, err
}
