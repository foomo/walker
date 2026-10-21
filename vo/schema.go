package vo

import "github.com/foomo/walker/htmlschema"

// SchemaRootInfo describes a schema root for the schema developer
type SchemaRootInfo struct {
	Groups     []string
	HasDefault bool
	// GroupHeader is the response header the walker reads a page's group from
	GroupHeader string
	LintErrors  []htmlschema.LintError
	Error       string
}

// SchemaValidation is a page validated by the schema developer
type SchemaValidation struct {
	URL   string
	Group string
	// Source is "server" for a page the server fetched, "rendered" for a snapshot of a browser's DOM
	Source     string
	StatusCode int
	// DetectedGroup is the value of the group header of the fetched page
	DetectedGroup string
	Report        *htmlschema.Report
	Trace         *htmlschema.TraceNode
	// Page is the head and the JSON-LD of the validated html
	Page       *htmlschema.PageData
	LintErrors []htmlschema.LintError
	Error      string
}
