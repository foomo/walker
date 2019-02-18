package walker

import (
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type Scraper struct {
}

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
	// duplication title, descr, h1
	// blocking robots txt
}

var ErrorNoBody = "no body"

func Scrape(targetURL string, chanResult chan ScrapeResult) {
	result := ScrapeResult{
		Code:      0,
		TargetURL: targetURL,
	}
	start := time.Now()
	resp, errGet := http.Get(targetURL)
	if errGet != nil {
		result.Error = errGet.Error()
		chanResult <- result
		return
	}
	result.Duration = time.Now().Sub(start)
	result.Code = resp.StatusCode
	result.Status = resp.Status
	if resp.Body == nil {
		result.Error = ErrorNoBody
		chanResult <- result
		return
	}

	result.ContentType = resp.Header.Get("Content-type")

	if strings.Contains(result.ContentType, "html") {

		doc, errNewDoc := goquery.NewDocumentFromResponse(resp)
		if errNewDoc != nil {
			result.Error = errNewDoc.Error()
			chanResult <- result
			return
		}

		linkList, errExtract := extractLinks(doc)
		resp.Body.Close()
		if errExtract != nil {
			result.Error = errExtract.Error()
			chanResult <- result
			return
		}
		result.Links = linkList

		structure, errExtractStructure := extractStructure(doc)
		if errExtractStructure != nil {
			result.Error = errExtractStructure.Error()
			return
		}
		result.Structure = structure
	} else {

	}
	chanResult <- result
	return
}

func extractLinks(doc *goquery.Document) (linkList LinkList, err error) {
	linkList = LinkList{}
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists && href != "" {
			linkList[href]++
		}
	})
	return
}
