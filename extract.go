package walker

import (
	"encoding/json"
	"fmt"

	"github.com/PuerkitoBio/goquery"
	"github.com/foomo/walker/vo"
)

func ExtractStructure(doc *goquery.Document) (s vo.Structure, err error) {
	description, _ := doc.Find("meta[name=description]").First().Attr("content")
	robots, _ := doc.Find("meta[name=robots]").First().Attr("content")
	s = vo.Structure{
		Title:       doc.Find("title").First().Text(),
		Description: description,
		Robots:      robots,
	}
	doc.Find("link[rel=prev], link[rel=next], link[rel=canonical]").Each(func(i int, sel *goquery.Selection) {
		attrRelVal, attrRelOK := sel.Attr("rel")
		attrHref, attrHrefOK := sel.Attr("href")
		if attrRelOK && attrHrefOK {
			switch attrRelVal {
			case "canonical":
				s.Canonical = attrHref
			case "prev":
				s.LinkPrev = attrHref
			case "next":
				s.LinkNext = attrHref
			}
		}
	})
	doc.Find("script[type=\"application/ld+json\"]").Each(func(i int, sel *goquery.Selection) {
		ld := &vo.LinkedData{}
		errDoc := json.Unmarshal([]byte(sel.Text()), &ld)
		if errDoc != nil {
			fmt.Println("json crap", errDoc)
			return
		}
		s.LinkedData = append(s.LinkedData, *ld)
	})
	doc.Find("h1, h2, h3, h4, h5, h6").Each(func(i int, sel *goquery.Selection) {
		level := 0
		switch sel.Get(0).Data {
		case "h1":
			level = 1
		case "h2":
			level = 2
		case "h3":
			level = 3
		case "h4":
			level = 4
		case "h5":
			level = 5
		case "h6":
			level = 6
		}
		s.Headings = append(s.Headings, vo.Heading{
			Level: level,
			Text:  sel.Text(),
		})
	})
	return
}
