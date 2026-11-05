package vo

import "github.com/foomo/walker/semanticscrape"

// SemanticDocument is the gist of a page, see semanticscrape.ExtractDocument
type SemanticDocument struct {
	URL string
	// Source is "server" for a page the server fetched, "rendered" for html
	// the client sent
	Source string
	// StatusCode of the fetched page, 0 for rendered html
	StatusCode int
	Document   *semanticscrape.Document
	Error      string
}
