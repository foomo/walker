package walker

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/foomo/walker/config"
)

type Service struct {
	Walker    *Walker
	targetURL string
}

func NewService(conf *config.Config) (s *Service, err error) {
	w := NewWalker(conf.Concurrency)
	w.walk(conf)
	s = &Service{
		Walker:    w,
		targetURL: conf.Target,
	}
	return
}

func filter(resultMap map[string]ScrapeResult, filterChain filterChain) {
	for targetURL, scrapeResult := range resultMap {
		for _, filterFunc := range filterChain {
			if !filterFunc(scrapeResult) {
				delete(resultMap, targetURL)
				break
			}
		}
	}
}

type filterFunc func(result ScrapeResult) bool
type filterChain []filterFunc

type Filters struct {
	Prefix string
	Status []int
	Errors []string
	MinDur time.Duration
	MaxDur time.Duration
}

type StatusStats struct {
	Code  int
	Count int
}

type FilterOptions struct {
	Status []StatusStats
	MinDur time.Duration
	MaxDur time.Duration
}

func getFilterChain(filters Filters) filterChain {
	chain := filterChain{}
	if filters.Prefix != "" {
		chain = append(chain, func(result ScrapeResult) bool {
			return strings.HasPrefix(result.TargetURL, filters.Prefix)
		})
	}
	if len(filters.Status) > 0 {
		chain = append(chain, func(result ScrapeResult) bool {
			for _, status := range filters.Status {
				if result.Code == status {
					return true
				}
			}
			return false
		})
	}
	if filters.MaxDur > 0 {
		chain = append(chain, func(result ScrapeResult) bool {
			return result.Duration < filters.MaxDur
		})
	}
	if filters.MinDur > 0 {
		chain = append(chain, func(result ScrapeResult) bool {
			return result.Duration > filters.MinDur
		})
	}
	return chain
}

func getFilterOptions(resultMap map[string]ScrapeResult) FilterOptions {
	statusMap := map[int]int{}
	minDur := time.Duration(1000000000000000000)
	maxDur := time.Duration(0)
	for _, result := range resultMap {
		statusMap[result.Code]++
		if result.Duration > maxDur {
			maxDur = result.Duration
		}
		if result.Duration < minDur {
			minDur = result.Duration
		}
	}
	statusStats := []StatusStats{}
	for code, count := range statusMap {
		statusStats = append(statusStats, StatusStats{
			Code:  code,
			Count: count,
		})
	}
	return FilterOptions{
		Status: statusStats,
		MinDur: minDur,
		MaxDur: maxDur,
	}
}

func (s *Service) GetResults(
	filters Filters,
	page int,
	pageSize int,
) (filterOptions FilterOptions, results []ScrapeResult, numPages int) {
	resultMap := s.Walker.GetStatus().Results

	filterOptions = getFilterOptions(resultMap)
	if filters.Prefix != "" {
		filters.Prefix = s.targetURL + filters.Prefix
	}
	filter(resultMap, getFilterChain(filters))

	i := 0
	results = make([]ScrapeResult, len(resultMap))

	urls := make([]string, len(resultMap))
	for url := range resultMap {
		urls[i] = url
		i++
	}
	sort.Strings(urls)
	for index, url := range urls {
		results[index] = resultMap[url]
	}
	// paging is missing
	numPages = len(results) / pageSize
	min := 0
	max := len(results)
	start := page * pageSize
	end := start + pageSize

	if start < min {
		start = min
	}
	if end > max {
		end = max
	}
	fmt.Println("slice", start, end)
	if end > start {
		results = results[start:end]
	}
	return
}

func (s *Service) GetStatus() ServiceStatus {
	walkerStatus := s.Walker.GetStatus()
	open := 0
	pending := 0
	for _, active := range walkerStatus.Jobs {
		if active {
			pending++
		} else {
			open++
		}
	}
	return ServiceStatus{
		TargetURL: s.targetURL,
		Done:      len(walkerStatus.Results),
		Open:      open,
		Pending:   pending,
	}
}
