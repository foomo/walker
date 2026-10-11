package vo

import "sort"

// PageLinks are the link counts of a scraped page
type PageLinks struct {
	TargetURL string
	Code      int
	Group     string
	Title     string
	// LinkedFrom is the number of pages linking to it - itself not counted
	LinkedFrom int
	// Links is the number of links to it, all pages together
	Links int
	// LinksTo is the number of distinct links on the page
	LinksTo int
}

// inboundLinks indexes the links of the results by target - self links are skipped
func inboundLinks(results map[string]ScrapeResult) map[string][]LinkSource {
	inbound := map[string][]LinkSource{}
	for _, r := range results {
		for link, count := range r.NormalizedLinks {
			if link == r.TargetURL {
				continue
			}
			if _, scraped := results[link]; scraped {
				inbound[link] = append(inbound[link], LinkSource{TargetURL: r.TargetURL, Group: r.Group, Count: count})
			}
		}
	}
	return inbound
}

// Links returns the link counts of every scraped page - sorted by url
func Links(results map[string]ScrapeResult) []PageLinks {
	inbound := inboundLinks(results)
	links := make([]PageLinks, 0, len(results))
	for targetURL, r := range results {
		pl := PageLinks{
			TargetURL:  r.TargetURL,
			Code:       r.Code,
			Group:      r.Group,
			Title:      r.Structure.Title,
			LinkedFrom: len(inbound[targetURL]),
			LinksTo:    len(r.NormalizedLinks),
		}
		for _, source := range inbound[targetURL] {
			pl.Links += source.Count
		}
		links = append(links, pl)
	}
	sort.Slice(links, func(i, j int) bool { return links[i].TargetURL < links[j].TargetURL })
	return links
}

// LinkedFromPage is a page of the pages linking to a url
type LinkedFromPage struct {
	// Total is the number of linking pages, Links the number of links
	Total int
	Links int
	// Groups breaks the linking pages down by group - the largest first
	Groups []LinkGroup
	// Sources are the first linking pages - the most links first
	Sources []LinkSource
}

// LinkGroup is the number of linking pages of a group
type LinkGroup struct {
	Group string
	Pages int
}

// LinkedFrom returns the pages linking to a target url - at most limit of
// them, limit <= 0 returns all
func LinkedFrom(results map[string]ScrapeResult, targetURL string, limit int) LinkedFromPage {
	page := LinkedFromPage{Groups: []LinkGroup{}, Sources: []LinkSource{}}
	groups := map[string]int{}
	for _, r := range results {
		if r.TargetURL == targetURL {
			continue
		}
		if count, ok := r.NormalizedLinks[targetURL]; ok {
			page.Sources = append(page.Sources, LinkSource{TargetURL: r.TargetURL, Group: r.Group, Count: count})
			page.Links += count
			groups[r.Group]++
		}
	}
	page.Total = len(page.Sources)
	for group, pages := range groups {
		page.Groups = append(page.Groups, LinkGroup{Group: group, Pages: pages})
	}
	sort.Slice(page.Groups, func(i, j int) bool {
		if page.Groups[i].Pages != page.Groups[j].Pages {
			return page.Groups[i].Pages > page.Groups[j].Pages
		}
		return page.Groups[i].Group < page.Groups[j].Group
	})
	sort.Slice(page.Sources, func(i, j int) bool {
		if page.Sources[i].Count != page.Sources[j].Count {
			return page.Sources[i].Count > page.Sources[j].Count
		}
		return page.Sources[i].TargetURL < page.Sources[j].TargetURL
	})
	if limit > 0 && len(page.Sources) > limit {
		page.Sources = page.Sources[:limit]
	}
	return page
}
