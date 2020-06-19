package reports

import (
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/foomo/walker/vo"
)

func normalizeCanonical(target, canonical string) string {
	if canonical == "" {
		return ""
	}
	targetURL, errParse := url.Parse(target)
	if errParse != nil {
		return ""
	}
	canonicalURL, errParse := url.Parse(canonical)
	if errParse != nil {
		return ""
	}

	if canonicalURL.Scheme != "" {
		return canonical
	}
	normalized := targetURL.Scheme + "//" + targetURL.Host
	port := targetURL.Port()
	if port != "" {
		normalized += ":" + port
	}
	normalized += canonical
	return normalized
}

func reportSEO(status vo.Status, w io.Writer, filter scrapeResultFilter) {
	printh, println, _ := printers(w)
	h1s := duplications{}
	titles := duplications{}
	descriptions := duplications{}
	missingTitles := uniqueList{}
	missingH1 := uniqueList{}
	missingDescriptions := uniqueList{}
	printh("SEO duplications")
	for _, r := range status.Results {
		if filter != nil && filter(r) == false {
			continue
		}
		finalURL := getFinalURLForScrapeResult(r)
		normalizedCanonical := normalizeCanonical(r.TargetURL, r.Structure.Canonical)
		if normalizedCanonical != finalURL {
			finalURL = normalizedCanonical
		}
		if strings.Contains(r.ContentType, "html") {
			foundH1 := false
			if r.Structure.Description != "" {
				descriptions.add(r.Structure.Description, finalURL)
			} else {
				missingDescriptions.add(finalURL)
			}
			for _, heading := range r.Structure.Headings {
				if r.Structure.Title == "" {
					missingTitles.add(finalURL)
				} else {
					titles.add(r.Structure.Title, finalURL)
				}
				if heading.Level == 1 && heading.Text != "" {
					h1s.add(heading.Text, finalURL)
					foundH1 = true
				}
			}
			if !foundH1 {
				missingH1.add(finalURL)
			}
		} else {
			fmt.Println(r.ContentType)
		}
	}
	printDuplicates := func(title string, d duplications) {
		if len(d) > 0 {
			printh(title)
			d.printlnDuplications(w)
		}
	}
	printDuplicates("duplicate h1", h1s)
	printDuplicates("duplicate titles", titles)
	printDuplicates("duplicate descriptions", descriptions)

	printList := func(name string, list []string) {
		if len(list) > 0 {
			printh(name)
			sort.Strings(list)
			for _, l := range list {
				println("	", l)
			}
		}
	}
	printList("missing titles", missingTitles)
	printList("missing descriptions", missingDescriptions)
	printList("missing h1", missingH1)
}
