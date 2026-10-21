package walker

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/foomo/walker/htmlschema"
	"github.com/foomo/walker/vo"
)

// the schema developer api - client and server share a file system, schema
// roots are paths on the server

const (
	SchemaSourceServer   = "server"
	SchemaSourceRendered = "rendered"
)

// GetSchemaRoot loads a schema root and lists its groups
func (s *service) GetSchemaRoot(schemaRoot string) (info vo.SchemaRootInfo) {
	start := time.Now()
	defer func() {
		logLintErrors(schemaRoot, info.LintErrors)
		attrs := []any{
			"root", schemaRoot,
			"groups", len(info.Groups),
			"default", info.HasDefault,
			"lintErrors", len(info.LintErrors),
			"duration", time.Since(start),
		}
		if info.Error != "" {
			slog.Warn("schema root failed", append(attrs, "error", info.Error)...)
			return
		}
		slog.Info("schema root loaded", attrs...)
	}()
	info.GroupHeader = s.groupHeader()
	root, lintErrors, err := loadSchemaRoot(schemaRoot)
	info.LintErrors = lintErrors
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.Groups = root.Groups()
	info.HasDefault = root.HasDefault()
	return info
}

// GetSchemaRootVersion is a fingerprint of the files of a schema root - it
// changes with every edit, the schema developer polls it to reload. Empty if
// the root can not be read.
func (s *service) GetSchemaRootVersion(schemaRoot string) string {
	if schemaRoot == "" {
		return ""
	}
	version, err := htmlschema.RootVersion(schemaRoot)
	if err != nil {
		return ""
	}
	return version
}

// ValidateSchema validates a page against a schema root, which is loaded on
// every call to pick up changes. Without html the server fetches targetURL,
// otherwise html is a snapshot of the page in a browser. An empty group is
// the group the page reports in the group header, or default.html.
func (s *service) ValidateSchema(schemaRoot, group, targetURL, html string) (validation vo.SchemaValidation) {
	var (
		start                       = time.Now()
		loadDuration, fetchDuration time.Duration
		validateDuration            time.Duration
		requestedGroup, htmlLength  = group, 0
	)
	defer func() {
		logLintErrors(schemaRoot, validation.LintErrors)
		attrs := []any{
			"root", schemaRoot,
			"url", targetURL,
			"source", validation.Source,
			"requestedGroup", requestedGroup,
			"group", validation.Group,
			"bytes", htmlLength,
			"load", loadDuration,
			"validate", validateDuration,
			"duration", time.Since(start),
		}
		if validation.Source == SchemaSourceServer {
			attrs = append(attrs, "status", validation.StatusCode, "detectedGroup", validation.DetectedGroup, "fetch", fetchDuration)
		}
		if validation.Error != "" {
			slog.Warn("schema validation failed", append(attrs, "lintErrors", len(validation.LintErrors), "error", validation.Error)...)
			return
		}
		r := validation.Report
		attrs = append(attrs,
			"schema", r.Schema,
			"points", r.Score.Points,
			"max", r.Score.Max,
			"penalty", r.Penalty,
			"errors", r.Count(htmlschema.SeverityError),
			"warnings", r.Count(htmlschema.SeverityWarn),
			"infos", r.Count(htmlschema.SeverityInfo),
		)
		slog.Info("schema validated", attrs...)
	}()
	validation.URL = targetURL
	validation.Source = SchemaSourceRendered
	loadStart := time.Now()
	root, lintErrors, err := loadSchemaRoot(schemaRoot)
	loadDuration = time.Since(loadStart)
	validation.LintErrors = lintErrors
	if err != nil {
		validation.Error = err.Error()
		return validation
	}
	htmlBytes := []byte(html)
	if html == "" {
		validation.Source = SchemaSourceServer
		fetchStart := time.Now()
		body, statusCode, detectedGroup, errFetch := s.fetch(targetURL)
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
	validation.Page, _ = htmlschema.ExtractPageData(htmlBytes)
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

// logLintErrors logs every problem of a schema root on debug level, the
// summaries only count them
func logLintErrors(schemaRoot string, lintErrors []htmlschema.LintError) {
	for _, e := range lintErrors {
		slog.Debug("schema lint error", "root", schemaRoot, "file", e.File, "line", e.Line, "message", e.Message)
	}
}

func loadSchemaRoot(schemaRoot string) (*htmlschema.Root, []htmlschema.LintError, error) {
	if schemaRoot == "" {
		return nil, nil, errors.New("no schema root")
	}
	root, err := htmlschema.LoadRoot(schemaRoot)
	var lintErrors htmlschema.LintErrors
	if errors.As(err, &lintErrors) {
		return nil, lintErrors, errors.New("the schema root has errors")
	}
	return root, nil, err
}

func (s *service) groupHeader() string {
	if s.conf == nil {
		return ""
	}
	return s.conf.GroupHeader
}

// fetch loads a page like the crawler does
func (s *service) fetch(targetURL string) (body []byte, statusCode int, group string, err error) {
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
	if header := s.groupHeader(); header != "" {
		group = resp.Header.Get(header)
		if strings.HasSuffix(group, "/") {
			group += "index"
		}
	}
	body, err = io.ReadAll(resp.Body)
	return body, resp.StatusCode, group, err
}
