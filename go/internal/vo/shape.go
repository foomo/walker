package vo

import "github.com/foomo/walker/go/internal/semanticshape"

// ShapeRootInfo describes a shape root for the shape developer
type ShapeRootInfo struct {
	Groups     []string
	HasDefault bool
	// GroupHeader is the response header the walker reads a page's group from
	GroupHeader string
	LintErrors  []semanticshape.LintError
	Error       string
}

// ShapeValidation is a page validated by the shape developer
type ShapeValidation struct {
	URL   string
	Group string
	// Source is "server" for a page the server fetched, "rendered" for a snapshot of a browser's DOM
	Source     string
	StatusCode int
	// DetectedGroup is the value of the group header of the fetched page
	DetectedGroup string
	Report        *semanticshape.Report
	Trace         *semanticshape.TraceNode
	// Page is the head and the JSON-LD of the validated html
	Page       *semanticshape.PageData
	LintErrors []semanticshape.LintError
	Error      string
}
