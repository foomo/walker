package htmlschema

import (
	"net/url"
	"regexp"

	"golang.org/x/net/html"
)

type AttributeRule interface {
	ValidateNode(n *html.Node) (valid bool, err error)
	Info() string
}

type attributeRuleLength struct {
	name        string
	info        string
	lengthMatch func(actual int) (bool, error)
}

func newAttributeRuleLength(attrName string, info string, lengthMatch func(actualLength int) (valid bool, err error)) AttributeRule {
	return &attributeRuleLength{
		name:        attrName,
		info:        info,
		lengthMatch: lengthMatch,
	}
}

func getAttrValue(n *html.Node, name string) string {
	for _, attr := range n.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func (rl *attributeRuleLength) ValidateNode(n *html.Node) (valid bool, err error) {
	return rl.lengthMatch(len(getAttrValue(n, rl.name)))
}
func (rl *attributeRuleLength) Info() string {
	return rl.info
}

type attributeRuleRegex struct {
	name  string
	regex *regexp.Regexp
}

func newattributeRuleRegex(name, regexString string) (rregex *attributeRuleRegex, err error) {
	regexString, errUnescape := url.QueryUnescape(regexString)
	if errUnescape != nil {
		return nil, errUnescape
	}
	regex, errParse := regexp.Compile(regexString)
	if errParse != nil {
		return nil, errParse
	}
	return &attributeRuleRegex{
		name:  name,
		regex: regex,
	}, nil
}

func (rregex *attributeRuleRegex) Info() string {
	return "regex :" + rregex.regex.String()
}
func (rregex *attributeRuleRegex) ValidateNode(n *html.Node) (valid bool, err error) {
	return rregex.regex.Match([]byte(getAttrValue(n, rregex.name))), nil
}
