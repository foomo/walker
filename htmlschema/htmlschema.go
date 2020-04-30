package htmlschema

type Attribute struct {
	Name  string
	Value string
	Rules map[string]AttributeRule
}

type Element struct {
	Name         string
	Content      string
	MinOccurence int
	Score        int
	MaxOccurence int
	Children     []*Element
	Source       string
	Attributes   []*Attribute
	MinLength    int
	MaxLength    int
	Selector     string
}

type Schema struct {
	Name     string
	Elements []*Element
}
