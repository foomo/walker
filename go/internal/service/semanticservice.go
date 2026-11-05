package service

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/foomo/walker/go/internal/semanticscrape"
	"github.com/foomo/walker/go/internal/vo"
)

// GetSemanticDocument extracts the gist of a page: language, title and
// description, and the semantic outline of its body - with content, also the
// texts, links, images and figures of its containers. Without html the server
// fetches targetURL (http or https) like the crawler does, otherwise html is
// the page, and targetURL only names it.
func (s *service) GetSemanticDocument(targetURL, html string, content bool) (result vo.SemanticDocument) {
	var (
		start         = time.Now()
		fetchDuration time.Duration
		htmlLength    int
	)
	defer func() {
		attrs := []any{
			"url", targetURL,
			"source", result.Source,
			"content", content,
			"bytes", htmlLength,
			"duration", time.Since(start),
		}
		if result.Source == ShapeSourceServer {
			attrs = append(attrs, "status", result.StatusCode, "fetch", fetchDuration)
		}
		if result.Error != "" {
			slog.Warn("semantic document failed", append(attrs, "error", result.Error)...)
			return
		}
		attrs = append(attrs, "title", result.Document.Title, "elements", len(result.Document.Body), "main", result.Document.Main() != nil)
		slog.Info("semantic document extracted", attrs...)
	}()
	result.URL = targetURL
	result.Source = ShapeSourceRendered
	htmlBytes := []byte(html)
	if html == "" {
		result.Source = ShapeSourceServer
		if err := checkPageURL(targetURL); err != nil {
			result.Error = err.Error()
			return result
		}
		fetchStart := time.Now()
		body, statusCode, _, err := s.fetch(targetURL, s.groupHeader())
		fetchDuration = time.Since(fetchStart)
		result.StatusCode = statusCode
		if err != nil {
			result.Error = err.Error()
			return result
		}
		// an error page would be extracted as if it was the page
		if statusCode < 200 || statusCode > 299 {
			result.Error = fmt.Sprintf("the page returned %d %s - nothing extracted", statusCode, http.StatusText(statusCode))
			return result
		}
		htmlBytes = body
	}
	htmlLength = len(htmlBytes)
	doc, err := semanticscrape.ExtractDocument(htmlBytes, semanticscrape.Options{Content: content})
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Document = doc
	return result
}

// checkPageURL accepts absolute http and https urls only - the server fetches
// them, other schemes have no business here
func checkPageURL(targetURL string) error {
	if targetURL == "" {
		return fmt.Errorf("neither a url nor html")
	}
	u, err := url.Parse(targetURL)
	if err != nil {
		return err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("not an http(s) url: %q", targetURL)
	}
	return nil
}
