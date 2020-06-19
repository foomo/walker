package vo

import (
	"time"

	"github.com/foomo/walker/htmlschema"
)

type LinkList map[string]int

type Redirect struct {
	Code int
	URL  string
}

type ScrapeResult struct {
	// index || noindex
	// <link rel="next" href="/damen/damentaschen/alle-taschen?page=2">
	// <meta name="robots" content="index,follow,noodp">
	TargetURL        string
	Redirects        []Redirect
	Error            string
	Code             int
	ValidationReport *htmlschema.Report
	ValidionError    error
	Status           string
	ContentType      string
	Length           int
	Links            LinkList
	Duration         time.Duration
	Time             time.Time
	Structure        Structure
	Validations      []Validation
	Data             interface{}
	Group            string
}
