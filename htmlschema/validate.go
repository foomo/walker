package htmlschema

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

type ValidationType string

const (
	// ValidationTypeOccurenceMismatch occurrence of an element is wrong
	ValidationTypeOccurenceMismatch ValidationType = "occurence-mismatch"
	// ValidationTypeContentLength length of content is wrong
	ValidationTypeContentLength ValidationType = "content-length"
	// ValidationTypeContent contains text, that is not allowed
	ValidationTypeContent ValidationType = "content"
	// ValidationTypeAttribute attribute is invalid
	ValidationTypeAttribute ValidationType = "attribute"
)

// Validation feedback from a validator
type Validation struct {
	Type    ValidationType
	Comment string
	Path    string
	Element *Element
	Penalty int
}

// Report of a validation of a html document with a schema
type Report struct {
	Score       int
	Validations []*Validation
}

// Print a report
func (r *Report) Print(w io.Writer) {
	p := &printer{w: w, indnt: 0}
	p.println("validation report", r.Score)
	p.println("------------------------------------------")
	totalPenalty := 0
	for _, v := range r.Validations {
		attrs := []string{}
		for _, attr := range v.Element.Attributes {
			attrs = append(attrs, attr.Name+":"+attr.Value)
		}
		p.println(v.Path, strings.Join(attrs, ","))
		p.indent(1)
		p.println(v.Type, ":", v.Comment)
		p.println("penalty:", v.Penalty)
		p.println(v.Element.Name, "score", v.Element.Score, "from", v.Element.Source)
		totalPenalty += v.Penalty
		p.indent(-1)
	}
	p.println("------------------------------------------")
	p.println("score			", r.Score)
	p.println("penalty		", totalPenalty)
	p.println("------------------------------------------")
	p.println("sum			", r.Score-totalPenalty)

}

type printer struct {
	w     io.Writer
	indnt int
}

func (p *printer) indent(inc int) {
	p.indnt += inc
}

func (p *printer) println(values ...interface{}) {
	if p.w == nil {
		return
	}
	values = append([]interface{}{strings.Repeat("	", p.indnt)}, values...)
	fmt.Fprintln(p.w, values...)
}

// ValidateURL validate a url including support fior file:// schmeme
func (s *Schema) ValidateURL(documentURL string, w io.Writer) (r *Report, err error) {
	var htmlBytes []byte
	if strings.HasPrefix(documentURL, "file://") {
		filename := strings.TrimPrefix(documentURL, "file://")
		filereader, errOpen := os.Open(filename)
		if errOpen != nil {
			return nil, errOpen
		}
		loadBytes, errLoad := ioutil.ReadAll(filereader)
		if errLoad != nil {
			return nil, errLoad
		}
		filereader.Close()
		htmlBytes = loadBytes
	} else {
		resp, errGet := http.Get(documentURL)
		if errGet != nil {
			return nil, errGet
		}
		if resp.StatusCode != http.StatusOK {
			return nil, errors.New(fmt.Sprint("unexpected response code: ", resp.StatusCode, ", status:", resp.Status))
		}
		defer resp.Body.Close()
		loadBytes, errLoad := ioutil.ReadAll(resp.Body)
		if errLoad != nil {
			return nil, errLoad
		}
		htmlBytes = loadBytes
	}
	return s.Validate(htmlBytes, w)
}

// Validate a html document
func (s *Schema) Validate(htmlBytes []byte, w io.Writer) (r *Report, err error) {
	doc, errParse := html.Parse(bytes.NewBuffer(htmlBytes))
	if errParse != nil {
		return nil, errParse
	}
	p := &printer{
		w:     w,
		indnt: 0,
	}
	p.println("using schema", s.Name)
	r = &Report{}
	for _, el := range s.Elements {
		el.validateNode(0, 1, doc, []string{}, r, p)
	}

	return
}

func (r *Report) addValidation(e *Element, path []string, t ValidationType, penalty int, blabla ...interface{}) {
	r.Validations = append(r.Validations, &Validation{
		Type:    t,
		Path:    strings.Join(path, "/"),
		Comment: fmt.Sprint(blabla...),
		Element: e,
		Penalty: penalty,
	})

}

func (e *Element) validateAttributes(path []string, r *Report, matchingNodes []*html.Node, p *printer) error {
	countAttrValidations := 0
	for _, attr := range e.Attributes {
		countAttrValidations += len(attr.Rules)
	}
	if countAttrValidations == 0 {
		return nil
	}
	for i, matchingNode := range matchingNodes {
		p.println("attributes", fmt.Sprint(e.Name, "[", i, "]"))
		for _, attr := range e.Attributes {
			p.indent(1)
			for ruleName, rule := range attr.Rules {
				ruleInfo := fmt.Sprint(e.Name, "[", i, "]@", attr.Name, " ", rule.Info())
				valid, err := rule.ValidateNode(matchingNode)
				if err != nil {
					return err
				}
				if !valid {
					p.println(ruleInfo, "not valid")
					r.addValidation(
						e,
						append(path, "@"+attr.Name),
						ValidationTypeAttribute,
						e.Score,
						"invalid attribute value with rule "+ruleName+":",
						getAttrValue(matchingNode, attr.Name),
					)
				} else {
					p.println(ruleInfo, "valid")
				}
			}
			p.indent(-1)
		}
	}
	return nil
}
func (e *Element) validateContentLength(path []string, r *Report, matchingNodes []*html.Node, p *printer) {
	if e.MinLength > -1 || e.MaxLength > -1 {
		for _, matchingNode := range matchingNodes {
			// we are expecting a length
			if matchingNode.FirstChild == nil || matchingNode.FirstChild.Type != html.TextNode {
				p.println("can not validate content length, there is no text")
				r.addValidation(
					e,
					path,
					ValidationTypeContent,
					e.Score,
					"wrong content type must be a text node",
				)
			} else {
				content := strings.Trim(matchingNode.FirstChild.Data, " 	\n")
				contentLength := len(content)
				result := "OK"
				if contentLength < e.MinLength {
					result = fmt.Sprint("content too short got ", contentLength, " expected ", e.MinLength)
					r.addValidation(
						e,
						path,
						ValidationTypeContent,
						e.Score,
						result,
					)
				}
				if e.MaxLength > -1 && contentLength > e.MaxLength {
					result = fmt.Sprint("content too long got ", contentLength, " expected ", e.MaxLength)
					r.addValidation(
						e,
						path,
						ValidationTypeContent,
						e.Score,
						result,
					)
				}
				p.println("checking length for", "\""+content+"\"", result)
			}
		}
	}
}

func dumpNode(n *html.Node, p *printer) {
	if p == nil {
		p = &printer{
			w:     os.Stdout,
			indnt: 0,
		}
	}
	if n.Type == html.ElementNode {
		p.println(n.Data)
		p.indent(1)
		for _, child := range getChildren(n) {
			dumpNode(child, p)
		}
		p.indent(-1)
	}
}

func (e *Element) getMatchingNodes(parentNode *html.Node) (matchingNodes []*html.Node, expectedAttributes map[string]string) {
	matchingNodes = []*html.Node{}
	expectedAttributes = map[string]string{}
	if e.Selector != "" && parentNode != nil {
		for _, selectorNode := range goquery.NewDocumentFromNode(parentNode).Find(e.Selector).Nodes {
			if selectorNode.Type == html.ElementNode {
				nextNode := &html.Node{
					FirstChild: selectorNode,
					Type:       html.ElementNode,
					Data:       "selectionRoot",
				}
				// dumpNode(nextNode, nil)
				matchingNodes = append(matchingNodes, nextNode)
			}
		}
		return matchingNodes, expectedAttributes
	}

	for _, attr := range e.Attributes {
		if len(attr.Rules) > 0 || strings.HasPrefix(attr.Name, "val:") {
			continue
		}
		expectedAttributes[attr.Name] = attr.Value
	}

SiblingLoop:
	for _, n := range getChildren(parentNode) {
		if n.Type == html.ElementNode && n.Data == e.Name {
			// node name matches
			for expectedAttrName, expectedAttrValue := range expectedAttributes {
				attrVal := getAttrValue(n, expectedAttrName)
				if attrVal != expectedAttrValue && !(expectedAttrValue == "*" && attrVal != "") {
					continue SiblingLoop
				}
			}
			matchingNodes = append(matchingNodes, n)
		}
	}
	return matchingNodes, expectedAttributes
}

func (e *Element) validateNodeOccurence(parentNode *html.Node, path []string, r *Report, p *printer) (matchingNodes []*html.Node) {
	matchingNodes, expectedAttributes := e.getMatchingNodes(parentNode)
	match := "not found"
	actualCount := len(matchingNodes)
	if parentNode != nil && actualCount > 0 {
		match = fmt.Sprint("found ", actualCount)
	}
	expectedAttributesInfos := []string{}
	for expectedAttributeName, expectedAttributeValue := range expectedAttributes {
		expectedAttributesInfos = append(expectedAttributesInfos, expectedAttributeName+"="+expectedAttributeValue)
	}
	expectation := "expecting(min=" + fmt.Sprint(e.MinOccurence) + ", max=" + fmt.Sprint(e.MaxOccurence) + ")"
	switch true {
	case e.Selector != "":
		expectation = "matching selector \"" + e.Selector + "\""
	case e.MaxOccurence == 0:
		expectation = "forbidden"
	case (e.MaxOccurence == e.MinOccurence) && e.MinOccurence > 0:
		expectation = "expecting(exactly=" + fmt.Sprint(e.MinOccurence) + ")"
	case e.MinOccurence == -1 && e.MaxOccurence == -1:
		expectation = "optional"
	}
	attrs := ""
	if len(expectedAttributesInfos) > 0 {
		attrs = "[" + strings.Join(expectedAttributesInfos, ",") + "]"
	}
	p.println("occurence"+attrs, expectation, match)

	countOK := true
	switch true {
	case e.Selector != "":
		// no validation here
	case e.MaxOccurence > -1 && actualCount > e.MaxOccurence:
		countOK = false
		r.addValidation(
			e,
			path,
			ValidationTypeOccurenceMismatch,
			e.Score,
			"too many elements of <", e.Name, "> got ", actualCount, " expected not more than ", e.MaxOccurence,
		)
	case actualCount < e.MinOccurence:
		countOK = false
		r.addValidation(
			e,
			path,
			ValidationTypeOccurenceMismatch,
			e.Score,
			"too few elements of <", e.Name, "> got ", actualCount, " expected at least ", e.MinOccurence,
		)
	}
	if countOK {
		r.Score += actualCount * e.Score
	}
	return matchingNodes
}

func (e *Element) validateNode(parentNodeIndex, parentNodeCount int, parentNode *html.Node, path []string, r *Report, p *printer) {
	nextPathElement := e.Name
	if e.Selector != "" {
		nextPathElement += "(" + e.Selector + ")"
	}
	suffix := ""
	switch parentNodeIndex {
	case -1:
		suffix = "[missing]"
	default:
		if parentNodeCount > 1 {
			suffix = "[" + fmt.Sprint(parentNodeIndex) + "]"
		}
	}
	path = append(path, nextPathElement+suffix)

	p.println("<" + e.Name + ">")

	p.indent(1)

	matchingNodes := e.validateNodeOccurence(parentNode, path, r, p)
	e.validateContentLength(path, r, matchingNodes, p)
	e.validateAttributes(path, r, matchingNodes, p)

	for _, childEl := range e.Children {
		// find childnode
		if len(matchingNodes) > 0 {
			for matchingNodeIndex, matchingNode := range matchingNodes {
				childEl.validateNode(matchingNodeIndex, len(matchingNodes), matchingNode, path, r, p)
			}
		} else if e.Selector == "" {
			childEl.validateNode(-1, -1, nil, path, r, p)
		} else if e.Selector != "" {
			// no matches for a selector
			/*
				fmt.Println("this should never happen " + e.Selector + " in " + strings.Join(path, "/"))
				spew.Dump(e)
			*/
		}
	}
	p.indent(-1)
}
