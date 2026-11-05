package vo

type Heading struct {
	Level int
	Text  string
}
type LinkedData struct {
	Context string `json:"@context"`
	Type    string ` json:"@type"`
}
type Structure struct {
	Title       string
	Description string
	Headings    []Heading
	Robots      string
	LinkedData  []LinkedData
	Canonical   string
	LinkPrev    string
	LinkNext    string
	// <link rel="prev" href="/herren/herrenmode/jacken">
	// <link rel="next" href="/herren/herrenmode/jacken?page=3">
	// <link rel="canonical" href="https://www.globus.ch/damen/damenmode/kleider">
}
