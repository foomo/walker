package walker

import (
	"net/url"
	"strings"

	"github.com/temoto/robotstxt"
)

func normalizeLink(baseURL *url.URL, linkURL string) (normalizedLink *url.URL, err error) {
	// let us ditch anchors
	anchorParts := strings.Split(linkURL, "#")
	linkURL = anchorParts[0]
	link, errParseLink := url.Parse(linkURL)
	if errParseLink != nil {
		err = errParseLink
		return
	}
	// host
	if link.Host == "" {
		link.Host = baseURL.Host
	}
	// scheme
	if link.Scheme == "" || link.Scheme == "//" {
		link.Scheme = baseURL.Scheme
	}
	if baseURL.User != nil {
		link.User = baseURL.User
	}
	// it is beautiful now
	normalizedLink = link
	return
}

func extractLinksToScrape(
	linkList LinkList,
	baseURL *url.URL,
	linkNextNormalized string,
	linkPrevNormalized string,
	ll linkLimitations,
	robotsGroup *robotstxt.Group,
) (links map[string]int) {
	links = map[string]int{}
LinkLoop:
	for linkURL := range linkList {
		// ok, time to really look at that url
		linkU, errParseLinkU := normalizeLink(baseURL, linkURL)
		if errParseLinkU == nil {

			// is it a pager link
			if !ll.paging {
				if linkNextNormalized == linkU.String() || linkPrevNormalized == linkU.String() {
					continue LinkLoop
				}
			}

			if linkU.Host != baseURL.Host || linkU.Scheme != baseURL.Scheme {
				// ignoring external links
				continue LinkLoop
			}

			if ll.depth > 0 {
				// too deep?
				if len(strings.Split(linkU.Path, "/"))-1 > ll.depth {
					continue LinkLoop
				}
			}

			// ignore path prefix
			for _, ignorePrefix := range ll.ignorePathPrefixes {
				if strings.HasPrefix(linkU.Path, ignorePrefix) {
					continue LinkLoop
				}
			}

			// robots say no
			if robotsGroup != nil && !robotsGroup.Test(linkU.Path) {
				continue LinkLoop
			}

			// are we ignoring it, because of the query
			if len(linkU.Query()) > 0 {
				// it has a query
				if ll.ignoreAllQueries {
					// no queries in general
					continue LinkLoop
				} else {
					// do we filter a query parameter
					for _, ignoreP := range ll.ignoreQueriesWith {
						for pName := range linkU.Query() {
							if pName == ignoreP {
								continue LinkLoop
							}
						}
					}
				}
			}

			// are we looking at the path is it included in the paths
			foundPath := false
			for _, p := range ll.includePathPrefixes {
				if strings.HasPrefix(linkU.Path, p) {
					foundPath = true
					break
				}
			}
			if !foundPath {
				// not in the scrape path
				continue LinkLoop
			}

			links[linkU.String()]++

		}
	}
	return links
}
