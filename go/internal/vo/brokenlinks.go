package vo

import "sort"

// IsBroken reports, if a status code means the page is broken - a failed
// request has code 0
func IsBroken(code int) bool {
	return code == 0 || code >= 400
}

// LinkSource is a page, that links to a broken page
type LinkSource struct {
	TargetURL string
	Group     string
	// Count is how often the page links to the broken page
	Count int
}

// BrokenLink is a scraped page, that is broken, and where it is linked from
type BrokenLink struct {
	TargetURL string
	Code      int
	Status    string
	Error     string
	Group     string
	// LinkedFrom are the pages linking to it, sorted by url
	LinkedFrom []LinkSource
	// Links is the number of links to it, all pages together
	Links int
}

// BrokenLinks finds the broken pages of the results and the pages linking to
// them - the most linked pages first
func BrokenLinks(results map[string]ScrapeResult) []BrokenLink {
	broken := map[string]*BrokenLink{}
	for targetURL, r := range results {
		if IsBroken(r.Code) {
			broken[targetURL] = &BrokenLink{
				TargetURL: r.TargetURL,
				Code:      r.Code,
				Status:    r.Status,
				Error:     r.Error,
				Group:     r.Group,
			}
		}
	}
	if len(broken) == 0 {
		return []BrokenLink{}
	}
	for _, r := range results {
		for link, count := range r.NormalizedLinks {
			if b, ok := broken[link]; ok {
				b.LinkedFrom = append(b.LinkedFrom, LinkSource{TargetURL: r.TargetURL, Group: r.Group, Count: count})
				b.Links += count
			}
		}
	}
	brokenLinks := make([]BrokenLink, 0, len(broken))
	for _, b := range broken {
		sort.Slice(b.LinkedFrom, func(i, j int) bool { return b.LinkedFrom[i].TargetURL < b.LinkedFrom[j].TargetURL })
		brokenLinks = append(brokenLinks, *b)
	}
	sort.Slice(brokenLinks, func(i, j int) bool {
		if len(brokenLinks[i].LinkedFrom) != len(brokenLinks[j].LinkedFrom) {
			return len(brokenLinks[i].LinkedFrom) > len(brokenLinks[j].LinkedFrom)
		}
		return brokenLinks[i].TargetURL < brokenLinks[j].TargetURL
	})
	return brokenLinks
}
