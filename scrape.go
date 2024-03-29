package walker

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/foomo/walker/htmlschema"
	"github.com/foomo/walker/vo"
)

var ErrorNoBody = "no body"

type scrapeResultAndClient struct {
	result     vo.ScrapeResult
	poolClient *poolClient
	doc        *goquery.Document
	docURL     *url.URL
}

func newScrapeResultandClient(r vo.ScrapeResult, pc *poolClient) scrapeResultAndClient {
	return scrapeResultAndClient{
		result:     r,
		poolClient: pc,
	}
}

func scrape(
	pc *poolClient,
	targetURL string,
	baseURL *url.URL,
	groupHeader string,
	scrapeFunc ScrapeFunc,
	validationFunc ValidationFunc,
	groupValidator *htmlschema.GroupValidator,
	chanResult chan scrapeResultAndClient,
) {
	result := vo.ScrapeResult{
		Code:      0,
		TargetURL: targetURL,
		Group:     "default",
	}
	var doc *goquery.Document
	start := time.Now()

	req, errRequest := http.NewRequest("GET", targetURL, nil)
	if errRequest != nil {
		result.Error = errRequest.Error()
		chanResult <- newScrapeResultandClient(result, pc)
		return
	}
	if baseURL.User != nil {
		req.URL.User = baseURL.User
	}
	req.Header.Set("User-Agent", pc.agent)
	req = req.WithContext(context.TODO())
	resp, errGet := pc.client.Do(req)
	if errGet != nil {
		result.Error = errGet.Error()
		chanResult <- newScrapeResultandClient(result, pc)
		return
	}
	result.Duration = time.Since(start)
	result.Code = resp.StatusCode
	result.Status = resp.Status
	result.Redirects = getRedirectsFromRequest(resp.Request)
	if resp.Body == nil {
		result.Error = ErrorNoBody
		chanResult <- newScrapeResultandClient(result, pc)
		return
	}

	result.ContentType = resp.Header.Get("Content-type")

	if groupHeader != "" {
		group := resp.Header.Get(groupHeader)
		if group != "" {
			result.Group = group
		}
		if strings.HasSuffix(result.Group, "/") {
			result.Group += "index"
		}
	}
	if strings.Contains(result.ContentType, "html") {
		bodyBytes, errReadAll := ioutil.ReadAll(resp.Body)
		if errReadAll != nil {
			result.Error = errReadAll.Error()
			chanResult <- newScrapeResultandClient(result, pc)
			return
		}
		resp.Body.Close()

		bodyReadCloser := ioutil.NopCloser(bytes.NewBuffer(bodyBytes))

		resp.Body = bodyReadCloser

		if groupValidator != nil {
			report, errValidate := groupValidator.Validate(result.Group, bodyBytes, nil)
			result.ValidationReport = report
			result.ValidionError = errValidate
		}

		nextDoc, errNewDoc := goquery.NewDocumentFromReader(bytes.NewBuffer(bodyBytes))
		if errNewDoc != nil {
			result.Error = errNewDoc.Error()
			chanResult <- newScrapeResultandClient(result, pc)
			return
		}
		doc = nextDoc

		linkList, normalizedLinkList, errExtract := extractLinks(doc, baseURL)
		if errExtract != nil {
			result.Error = errExtract.Error()
			chanResult <- newScrapeResultandClient(result, pc)
			return
		}
		result.Links = linkList
		result.NormalizedLinks = normalizedLinkList

		structure, errExtractStructure := ExtractStructure(doc)
		if errExtractStructure != nil {
			result.Error = errExtractStructure.Error()
			return
		}
		result.Structure = structure

		if scrapeFunc != nil {
			customScrapeData, errScrape := scrapeFunc(resp)
			if errScrape != nil {
				result.Error = errScrape.Error()
				chanResult <- newScrapeResultandClient(result, pc)
				return
			}
			result.Data = customScrapeData
		}

		if validationFunc != nil {
			validations, errValidate := validationFunc(result.Structure, result.Data)
			if errValidate != nil {
				result.Error = errValidate.Error()
				return
			}
			result.Validations = validations
		}

	}

	r := newScrapeResultandClient(result, pc)
	r.doc = doc
	r.docURL = req.URL
	chanResult <- r
}

func extractLinks(doc *goquery.Document, baseURL *url.URL) (linkList, normalizedLinkList vo.LinkList, err error) {
	linkList = vo.LinkList{}
	firstCanonical := doc.Find("link[rel=canonical]").First()
	if firstCanonical != nil {
		canonicalHref, existsCanonicalHref := firstCanonical.Attr("href")
		if existsCanonicalHref {
			linkList[canonicalHref]++
		}
	}
	handleA := func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists && href != "" {
			linkList[href]++
		}
	}
	doc.Find("a").Each(handleA)
	doc.Find("noscript").Each(func(i int, s *goquery.Selection) {
		doc, errNewDoc := goquery.NewDocumentFromReader(bytes.NewBuffer([]byte(s.Text())))
		if errNewDoc == nil {
			doc.Find("a").Each(handleA)
		}
	})

	normalizedLinkList = vo.LinkList{}
	for l, c := range linkList {
		nl, errNormalize := NormalizeLink(baseURL, l)
		if errNormalize == nil {
			normalizedLinkList[nl.String()] = c
		}
	}
	return
}
