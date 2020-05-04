package htmlschema

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// Load a file
func Load(file string) (schema *Schema, err error) {
	return load(file, nil)
}

func load(file string, context *html.Node) (schema *Schema, err error) {
	reader, errOpen := os.Open(file)
	if errOpen != nil {
		return nil, errOpen
	}
	defer reader.Close()
	var errLoad error
	var doc *html.Node
	var childNodes []*html.Node
	if context != nil {
		childNodes, errLoad = html.ParseFragment(reader, context)
	} else {
		doc, errLoad = html.Parse(reader)
	}
	if errLoad != nil {
		return nil, errLoad
	}
	if doc != nil {
		childNodes = getChildren(doc)
	}
	schema = &Schema{
		Name: file,
	}
	for _, n := range childNodes {
		el, errLoadElement := NewElementFromNode(n, file)
		if errLoadElement != nil {
			return nil, errors.New("error in file " + file + ": ," + errLoadElement.Error())
		}
		if el == nil {
			continue
		}
		schema.Elements = append(schema.Elements, el)
	}
	return
}

func NewElementFromNode(n *html.Node, source string) (el *Element, err error) {
	switch n.Type {
	case html.ElementNode:
		el := &Element{
			Name:         n.Data,
			Source:       source,
			MinOccurence: -1,
			MaxOccurence: -1,
			MinLength:    -1,
			MaxLength:    -1,
		}
		errLoadAttributes := el.loadAttributes(n)
		if errLoadAttributes != nil {
			return nil, errLoadAttributes
		}
		switch el.Name {
		case "val:selector":
			el.Selector = getAttrValue(n, "selector")
			if el.Selector == "" {
				return nil, errors.New(`<val:selector selector="must not be empty">`)
			}
		case "ref":
			// load referenced schema and merge it
			if n.FirstChild == nil || n.FirstChild.Data == "" {
				return nil, errors.New("can not load empty ref")
			}
			refFile := n.FirstChild.Data
			if !filepath.IsAbs(refFile) {
				refFile = filepath.Join(filepath.Dir(source), refFile)
			}
			refSchema, errLoadRefSchema := load(refFile, n)
			if errLoadRefSchema != nil {
				return nil, errors.New("could not load nested schema from ref: " + errLoadRefSchema.Error())
			}
			if len(refSchema.Elements) != 1 {
				return nil, errors.New("when loading a sub schema, it must have exactyle one top level element")
			}
			return refSchema.Elements[0], nil
		}
		for _, childNode := range getChildren(n) {
			childEl, errLoadChildEl := NewElementFromNode(childNode, source)
			if errLoadChildEl != nil {
				return nil, errLoadChildEl
			}
			if childEl != nil {
				el.Children = append(el.Children, childEl)
			}
		}
		return el, nil
	default:
		return nil, nil
	}
}

func (el *Element) loadAttributes(n *html.Node) error {
	occurenceWasSet := false
	for _, a := range n.Attr {
		intVal, errIntVal := strconv.Atoi(a.Val)
		switch a.Key {
		case "val:score":
			if errIntVal != nil {
				return errIntVal
			}
			el.Score = intVal
		case "val:min":
			if errIntVal != nil {
				return errIntVal
			}
			occurenceWasSet = true
			el.MinOccurence = intVal
		case "val:max":
			if errIntVal != nil {
				return errIntVal
			}
			occurenceWasSet = true
			el.MaxOccurence = intVal
		case "val:optional":
			occurenceWasSet = true
			el.MinOccurence = 0
		case "val:forbidden":
			occurenceWasSet = true
			el.MaxOccurence = 0
		case "val:min-length":
			if errIntVal != nil {
				return errIntVal
			}
			el.MinLength = intVal
		case "val:max-length":
			if errIntVal != nil {
				return errIntVal
			}
			el.MaxLength = intVal
		case "val:count":
			if errIntVal != nil {
				return errIntVal
			}
			occurenceWasSet = true
			el.MaxOccurence = intVal
			el.MinOccurence = intVal
		case "val:attr":
			//var ruleMinLength *attributeRuleMinLength
			parts := strings.Split(a.Val, ";")

			attr := &Attribute{
				Name:  "",
				Value: a.Val,
				Rules: map[string]AttributeRule{},
			}
			for i, part := range parts {
				part = strings.Trim(part, " 	\n")
				if i == 0 {
					attr.Name = part
					continue
				}
				ruleParts := strings.Split(part, ":")
				if len(ruleParts) == 2 {
					ruleName := strings.Trim(ruleParts[0], "	 ")
					ruleData := strings.Trim(ruleParts[1], "	 ")
					ruleDataInt, errRuleDataInt := strconv.Atoi(ruleData)
					switch ruleName {
					case "regex":
						ruleRegex, errLoadRule := newattributeRuleRegex(attr.Name, ruleData)
						if errLoadRule != nil {
							return errLoadRule
						}
						attr.Rules[ruleName] = ruleRegex
					case "min-length", "length", "max-length":
						if errRuleDataInt != nil {
							return errRuleDataInt
						}
						attr.Rules[ruleName] = newAttributeRuleLength(attr.Name, ruleName, func(actual int) (vaild bool, err error) {
							switch ruleName {
							case "min-length":
								return actual > ruleDataInt, nil
							case "length":
								return actual == ruleDataInt, nil
							case "max-length":
								return actual < ruleDataInt, nil
							default:
								return false, errors.New("unknown rule" + ruleName)
							}
						})
					default:
					}
				}
			}
			if attr.Name != "" && len(attr.Rules) > 0 {
				el.Attributes = append(el.Attributes, attr)
			}

		default:
			el.Attributes = append(el.Attributes, &Attribute{Name: a.Key, Value: a.Val})
		}
	}
	if el.MaxOccurence > -1 && el.MinOccurence > el.MaxOccurence {
		return errors.New("it does not make sense, if el.MinOccurence > el.MaxOccurence ... for " + el.Name + " in " + el.Source)
	}
	if !occurenceWasSet {
		el.MinOccurence = 1
		el.MaxOccurence = 1
	}
	return nil
}

func getChildren(p *html.Node) (children []*html.Node) {
	if p == nil {
		return
	}
	children = []*html.Node{}
	child := p.FirstChild
	for child != nil {
		children = append(children, child)
		child = child.NextSibling
	}
	return
}
