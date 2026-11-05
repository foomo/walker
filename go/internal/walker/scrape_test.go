package walker

import (
	"bytes"
	_ "embed"
	"fmt"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/davecgh/go-spew/spew"
)

//go:embed "test.html"
var testDoc []byte

func TestScrape(t *testing.T) {

	doc, errNewDoc := goquery.NewDocumentFromReader(bytes.NewBuffer(testDoc))
	spew.Dump(errNewDoc)

	doc.Find("noscript").Each(func(i int, s *goquery.Selection) {
		fmt.Println(s.Text())
		doc, errNewDoc := goquery.NewDocumentFromReader(bytes.NewBuffer([]byte(s.Text())))
		if errNewDoc == nil {
			doc.Find("a").Each(func(i int, s *goquery.Selection) {
				href, exists := s.Attr("href")
				fmt.Println(i, exists, href)
			})
		}

	})

}
