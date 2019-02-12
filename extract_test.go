package walker

import (
	"bytes"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
)

const testDocHTML = `
<html>
<head>
	<title>Hello Test</title>
	<meta name="description" content="this is a test doc and i am a description">
	<link rel="canonical" href="https://www.globus.ch/damen/damenmode/kleider">	
	<link rel="prev" href="/herren/herrenmode/jacken">
	<link rel="next" href="/herren/herrenmode/jacken?page=3">
</head>
<body>
<h1>h1-0</h1>
<h2>h2-0</h2>
<h2>h2-1</h2>
<h3>h3-0</h3>
<h1>h1-1</h1>
<h2>h2-2</h2>
<script type="application/ld+json">{"@context":"http://schema.org","@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"item":{"@id":"/","name":"Globus"}},{"@type":"ListItem","position":2,"item":{"@id":"/home-living","name":"Home & Living"}},{"@type":"ListItem","position":3,"item":{"@id":"/home-living/weihnachten","name":"Weihnachten"}},{"@type":"ListItem","position":4,"item":{"@id":"/home-living/weihnachten/baumschmuck","name":"Baumschmuck"}},{"@type":"ListItem","position":5,"item":{"@id":"/globus-baumschmuck-schwan-bu0937790012000","name":"Baumschmuck SCHWAN"}}]}</script>
<script type="application/ld+json">{"@context":"http://schema.org/","@type":"Product","name":"Baumschmuck SCHWAN","image":"https://www.globus.ch/media/shop/1894881/1538742072000","color":"weiss","brand":{"@type":"Thing","name":"GLOBUS"},"description":"Ist mundgeblasen, Ist handbemalt, Made in Europe","offers":[{"@type":"Offer","sku":"1257706000","price":"12.90","category":"Globus > Home & Living > Weihnachten > Baumschmuck","priceCurrency":"CHF","itemCondition":"NewCondition","availability":"InStock"}]}</script>
</body>
</html>
`

const emptyDocHTML = ``

func getDoc(t *testing.T, html string) *goquery.Document {
	htmlReader := bytes.NewReader([]byte(html))
	doc, errDoc := goquery.NewDocumentFromReader(htmlReader)
	assert.Nil(t, errDoc)
	return doc
}

func TestExtract(t *testing.T) {
	testDoc := getDoc(t, testDocHTML)
	emptyDoc := getDoc(t, emptyDocHTML)
	testDocStructure, eStucture := extractStructure(testDoc)
	t.Log(testDocStructure, eStucture)
	spew.Dump(testDocStructure)
	emptyDocStructure, eStucture := extractStructure(emptyDoc)
	t.Log(emptyDocStructure, eStucture)
}
