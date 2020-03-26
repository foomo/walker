package vo

import "time"

type LinkList map[string]int

type ScrapeResult struct {
	// index || noindex
	// <link rel="next" href="/damen/damentaschen/alle-taschen?page=2">
	// <meta name="robots" content="index,follow,noodp">
	TargetURL   string
	Error       string
	Code        int
	Status      string
	ContentType string
	Length      int
	Links       LinkList
	Duration    time.Duration
	Time        time.Time
	Structure   Structure
	Group       string
	// duplication title, descr, h1
	// blocking robots txt
}
