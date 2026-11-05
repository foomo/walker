package walker

import (
	"github.com/foomo/walker/go/internal/vo"
)

// Service is the api of the walker service - see the methods of
// service for the details
type Service interface {
	// results
	GetResults(filters Filters, page int, pageSize int) (filterOptions FilterOptions, results []vo.ScrapeResult, numPages int, folders []vo.Folder)
	GetResult(scrape string, targetURL string) *vo.ScrapeResult

	// links
	GetBrokenLinks(scrape string) []vo.BrokenLink
	GetRedirects(scrape string) []vo.RedirectChain
	GetLinks(scrape string) []vo.PageLinks
	GetLinkedFrom(scrape string, targetURL string, limit int) vo.LinkedFromPage

	// graph
	GetGraph(scrape string, options vo.GraphOptions) vo.Graph
	GetPageGraph(scrape string, options vo.GraphOptions, page string, maxNodes int) vo.Graph
	GetPageFolderLinks(scrape string, options vo.GraphOptions, page string) []vo.FolderLink
	GetFolderPages(scrape string, options vo.GraphOptions, folder string, maxPages int) vo.FolderPages

	// scrapes
	GetScrapes() (complete, current *vo.ScrapeSummary)
	RestartScrape() bool
	GetStatus() vo.ServiceStatus

	// schema developer
	GetSchemaRoot(schemaRoot string) (info vo.SchemaRootInfo)
	GetSchemaRootVersion(schemaRoot string) string
	ValidateSchema(schemaRoot, group, targetURL, html string) (validation vo.SchemaValidation)

	// semantic scrape
	GetSemanticDocument(targetURL, html string, content bool) (result vo.SemanticDocument)
}

var _ Service = (*service)(nil)
