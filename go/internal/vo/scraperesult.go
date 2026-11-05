package vo

import (
	"time"

	"github.com/foomo/walker/go/internal/semanticshape"
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
	ValidationReport *semanticshape.Report
	ValidationError  string
	Status           string
	ContentType      string
	Length           int
	Links            LinkList
	NormalizedLinks  LinkList
	// Duration is the time until the response headers arrived, redirects included.
	Duration time.Duration
	// TTFB (time to first byte) of the final response: from the start of its
	// request, connection setup included, to the first byte of the response.
	TTFB        time.Duration
	Time        time.Time
	Structure   Structure
	Validations []ResultValidation
	Data        interface{}
	Group       string
}
