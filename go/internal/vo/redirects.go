package vo

import "sort"

// RedirectChain is a scraped url, that redirects, and where it is linked from
type RedirectChain struct {
	TargetURL string
	Group     string
	// Hops are the redirects in order - the code of the response and the url it points to
	Hops []Redirect
	// FinalURL and FinalCode are where the chain ends
	FinalURL  string
	FinalCode int
	// Permanent is true, if every hop is a 301 or 308
	Permanent bool
	// LinkedFrom are the pages linking to the redirecting url, sorted by url
	LinkedFrom []LinkSource
	// Links is the number of links to it, all pages together
	Links int
}

// Redirects finds the redirecting urls of the results and the pages linking
// to them - the longest chains first, then the most linked
func Redirects(results map[string]ScrapeResult) []RedirectChain {
	chains := map[string]*RedirectChain{}
	for targetURL, r := range results {
		if len(r.Redirects) == 0 {
			continue
		}
		permanent := true
		for _, hop := range r.Redirects {
			permanent = permanent && (hop.Code == 301 || hop.Code == 308)
		}
		chains[targetURL] = &RedirectChain{
			TargetURL: r.TargetURL,
			Group:     r.Group,
			Hops:      r.Redirects,
			FinalURL:  r.Redirects[len(r.Redirects)-1].URL,
			FinalCode: r.Code,
			Permanent: permanent,
		}
	}
	if len(chains) == 0 {
		return []RedirectChain{}
	}
	for _, r := range results {
		for link, count := range r.NormalizedLinks {
			if c, ok := chains[link]; ok {
				c.LinkedFrom = append(c.LinkedFrom, LinkSource{TargetURL: r.TargetURL, Group: r.Group, Count: count})
				c.Links += count
			}
		}
	}
	redirects := make([]RedirectChain, 0, len(chains))
	for _, c := range chains {
		sort.Slice(c.LinkedFrom, func(i, j int) bool { return c.LinkedFrom[i].TargetURL < c.LinkedFrom[j].TargetURL })
		redirects = append(redirects, *c)
	}
	sort.Slice(redirects, func(i, j int) bool {
		a, b := redirects[i], redirects[j]
		if len(a.Hops) != len(b.Hops) {
			return len(a.Hops) > len(b.Hops)
		}
		if len(a.LinkedFrom) != len(b.LinkedFrom) {
			return len(a.LinkedFrom) > len(b.LinkedFrom)
		}
		return a.TargetURL < b.TargetURL
	})
	return redirects
}
