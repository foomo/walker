package vo

import (
	"sort"
	"time"
)

// GraphOptions shape the graph - zero values are the defaults
type GraphOptions struct {
	// Depth is the deepest folder level of the folder graph - the host is 0, default 2
	Depth int
	// GlobalShare - a target linked from more than this share of the pages is
	// a navigation target, links to it are dropped - default 0.5
	GlobalShare float64
}

func (o GraphOptions) withDefaults() GraphOptions {
	if o.Depth <= 0 {
		o.Depth = 2
	}
	if o.GlobalShare <= 0 || o.GlobalShare > 1 {
		o.GlobalShare = 0.5
	}
	return o
}

// Graph of the folders of the scraped site - edges refer to nodes by index,
// to keep it compact
type Graph struct {
	Nodes []GraphNode
	Edges []GraphEdge
	// Global are the navigation targets, links to them are not edges
	Global []GlobalLink
}

// FolderPages are the pages of one folder node of the graph
type FolderPages struct {
	Pages []GraphNode
	// Edges are the links between the pages - indices into Pages
	Edges []GraphEdge
	// Dropped is the number of pages left out - only the most linked are kept
	Dropped int
}

// GraphNode is a folder or a page
type GraphNode struct {
	// ID is the folder prefix or the page url
	ID string
	// Label is the folder name or the page path
	Label string
	// Pages and Errors - a folder counts the pages below it, a page is 1
	Pages  int
	Errors int
	// Code is the status of a page - of a folder the worst of its pages
	Code int
	// Group of a page - of a folder the most common one
	Group string
	// P90 is the p90 duration of a folder - of a page its duration
	P90 time.Duration
	// LinkedFrom is the number of pages linking to it - for a folder from
	// outside the folder
	LinkedFrom int
}

// GraphEdge are the links from one node to another
type GraphEdge struct {
	From  int
	To    int
	Links int
}

// GlobalLink is a navigation target and how many pages link to it
type GlobalLink struct {
	URL        string
	LinkedFrom int
}

// severity orders status codes from fine to worst - a failed request is worst
func severity(code int) int {
	switch {
	case code == 0:
		return 6
	case code >= 500:
		return 5
	case code >= 400:
		return 4
	case code >= 300:
		return 3
	case code >= 200:
		return 2
	default:
		return 1
	}
}

// BuildGraph builds the folder graph of the results down to options.Depth
func BuildGraph(results map[string]ScrapeResult, options GraphOptions) Graph {
	options = options.withDefaults()
	inbound := inboundLinks(results)
	global, globalLinks := globalTargets(results, inbound, options.GlobalShare)
	graph := Graph{Nodes: []GraphNode{}, Global: globalLinks}
	// nodeOf maps a target url to its node
	nodeOf := map[string]int{}
	folderNodes(&graph, results, inbound, options.Depth, nodeOf)
	graph.Edges = edgesBetween(results, nodeOf, global)
	return graph
}

// BuildFolderPages returns the pages of a folder node of the graph - the
// pages the node stands for, the maxPages most linked of them
func BuildFolderPages(results map[string]ScrapeResult, options GraphOptions, folder string, maxPages int) FolderPages {
	options = options.withDefaults()
	if maxPages <= 0 {
		maxPages = 200
	}
	inbound := inboundLinks(results)
	global, _ := globalTargets(results, inbound, options.GlobalShare)
	tree := folderTree(results)

	targetURLs := []string{}
	for targetURL, r := range results {
		if node := folderAt(tree, r.TargetURL, options.Depth); node != nil && node.folder.Prefix == folder {
			targetURLs = append(targetURLs, targetURL)
		}
	}
	sort.Slice(targetURLs, func(i, j int) bool {
		a, b := len(inbound[targetURLs[i]]), len(inbound[targetURLs[j]])
		if a != b {
			return a > b
		}
		return targetURLs[i] < targetURLs[j]
	})
	pages := FolderPages{Pages: []GraphNode{}}
	if len(targetURLs) > maxPages {
		pages.Dropped = len(targetURLs) - maxPages
		targetURLs = targetURLs[:maxPages]
	}
	sort.Strings(targetURLs)
	nodeOf := map[string]int{}
	for _, targetURL := range targetURLs {
		nodeOf[targetURL] = len(pages.Pages)
		pages.Pages = append(pages.Pages, pageNode(results[targetURL], len(inbound[targetURL])))
	}
	pages.Edges = edgesBetween(results, nodeOf, global)
	return pages
}

// BuildPageGraph returns the neighborhood of a page: the page itself as the
// first node, the pages linking to it and the pages it links to - the
// maxNodes most linked of them - and the links among all of them, without
// navigation links
func BuildPageGraph(results map[string]ScrapeResult, options GraphOptions, page string, maxNodes int) Graph {
	options = options.withDefaults()
	if maxNodes <= 0 {
		maxNodes = 120
	}
	inbound := inboundLinks(results)
	global, globalLinks := globalTargets(results, inbound, options.GlobalShare)
	graph := Graph{Nodes: []GraphNode{}, Edges: []GraphEdge{}, Global: globalLinks}
	root, ok := results[page]
	if !ok {
		return graph
	}

	// the neighbors - linking in and linked to, navigation targets left out
	neighbors := map[string]bool{}
	for _, source := range inbound[page] {
		neighbors[source.TargetURL] = true
	}
	for link := range root.NormalizedLinks {
		if _, scraped := results[link]; scraped && !global[link] {
			neighbors[link] = true
		}
	}
	delete(neighbors, page)
	targetURLs := make([]string, 0, len(neighbors))
	for targetURL := range neighbors {
		targetURLs = append(targetURLs, targetURL)
	}
	sort.Slice(targetURLs, func(i, j int) bool {
		a, b := len(inbound[targetURLs[i]]), len(inbound[targetURLs[j]])
		if a != b {
			return a > b
		}
		return targetURLs[i] < targetURLs[j]
	})
	if len(targetURLs) > maxNodes-1 {
		targetURLs = targetURLs[:maxNodes-1]
	}
	sort.Strings(targetURLs)

	nodeOf := map[string]int{page: 0}
	graph.Nodes = append(graph.Nodes, pageNode(root, len(inbound[page])))
	for _, targetURL := range targetURLs {
		nodeOf[targetURL] = len(graph.Nodes)
		graph.Nodes = append(graph.Nodes, pageNode(results[targetURL], len(inbound[targetURL])))
	}
	// the page itself may be a navigation target - links to it still count here
	delete(global, page)
	graph.Edges = edgesBetween(results, nodeOf, global)
	return graph
}

// FolderLink are the links from a page to a folder node of the graph
type FolderLink struct {
	// Folder is the index of the folder in Graph.Nodes
	Folder int
	Links  int
}

// BuildPageFolderLinks returns the links of a page to the folder nodes of the
// graph built with the same options - without navigation links and without
// links into the page's own folder, sorted by folder
func BuildPageFolderLinks(results map[string]ScrapeResult, options GraphOptions, page string) []FolderLink {
	options = options.withDefaults()
	links := []FolderLink{}
	r, ok := results[page]
	if !ok {
		return links
	}
	inbound := inboundLinks(results)
	global, _ := globalTargets(results, inbound, options.GlobalShare)
	nodeOf := map[string]int{}
	folderNodes(&Graph{}, results, inbound, options.Depth, nodeOf)
	own := nodeOf[page]
	counts := map[int]int{}
	for link, count := range r.NormalizedLinks {
		folder, ok := nodeOf[link]
		if !ok || global[link] || folder == own {
			continue
		}
		counts[folder] += count
	}
	for folder, count := range counts {
		links = append(links, FolderLink{Folder: folder, Links: count})
	}
	sort.Slice(links, func(i, j int) bool { return links[i].Folder < links[j].Folder })
	return links
}

func pageNode(r ScrapeResult, linkedFrom int) GraphNode {
	node := GraphNode{
		ID:         r.TargetURL,
		Label:      URLPath(r.TargetURL),
		Pages:      1,
		Code:       r.Code,
		Group:      r.Group,
		P90:        r.Duration,
		LinkedFrom: linkedFrom,
	}
	if node.Label == "" {
		node.Label = "/"
	}
	if IsBroken(r.Code) {
		node.Errors = 1
	}
	return node
}

// globalTargets finds the navigation targets - linked from more than share of the pages
func globalTargets(results map[string]ScrapeResult, inbound map[string][]LinkSource, share float64) (map[string]bool, []GlobalLink) {
	global := map[string]bool{}
	links := []GlobalLink{}
	for target, sources := range inbound {
		if float64(len(sources)) > share*float64(len(results)) {
			global[target] = true
			links = append(links, GlobalLink{URL: results[target].TargetURL, LinkedFrom: len(sources)})
		}
	}
	sort.Slice(links, func(i, j int) bool {
		if links[i].LinkedFrom != links[j].LinkedFrom {
			return links[i].LinkedFrom > links[j].LinkedFrom
		}
		return links[i].URL < links[j].URL
	})
	return global, links
}

// edgesBetween sums the links between the nodes - without navigation links
// and self loops; nodeOf maps target urls to node indices
func edgesBetween(results map[string]ScrapeResult, nodeOf map[string]int, global map[string]bool) []GraphEdge {
	type key struct{ from, to int }
	links := map[key]int{}
	for targetURL, r := range results {
		from, ok := nodeOf[targetURL]
		if !ok {
			continue
		}
		for link, count := range r.NormalizedLinks {
			to, ok := nodeOf[link]
			if !ok || global[link] || to == from {
				continue
			}
			links[key{from, to}] += count
		}
	}
	edges := make([]GraphEdge, 0, len(links))
	for k, count := range links {
		edges = append(edges, GraphEdge{From: k.from, To: k.to, Links: count})
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
	return edges
}

// folderNodes adds a node per folder down to depth - every page belongs to
// the deepest of them, that contains it
func folderNodes(graph *Graph, results map[string]ScrapeResult, inbound map[string][]LinkSource, depth int, nodeOf map[string]int) {
	tree := folderTree(results)
	type folder struct {
		node      GraphNode
		durations []time.Duration
		groups    map[string]int
		linked    map[string]bool
	}
	folders := map[string]*folder{}
	for targetURL, r := range results {
		treeNode := folderAt(tree, r.TargetURL, depth)
		if treeNode == nil {
			continue
		}
		prefix := treeNode.folder.Prefix
		f, ok := folders[prefix]
		if !ok {
			f = &folder{
				node:   GraphNode{ID: prefix, Label: treeNode.folder.Name, Code: 200},
				groups: map[string]int{},
				linked: map[string]bool{},
			}
			folders[prefix] = f
		}
		f.node.Pages++
		if IsBroken(r.Code) {
			f.node.Errors++
		}
		if severity(r.Code) > severity(f.node.Code) {
			f.node.Code = r.Code
		}
		f.durations = append(f.durations, r.Duration)
		f.groups[r.Group]++
		for _, source := range inbound[targetURL] {
			f.linked[source.TargetURL] = true
		}
		nodeOf[targetURL] = -1 // resolved below, once the folders are sorted
	}

	prefixes := make([]string, 0, len(folders))
	for prefix := range folders {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	index := map[string]int{}
	for _, prefix := range prefixes {
		f := folders[prefix]
		sort.Slice(f.durations, func(i, j int) bool { return f.durations[i] < f.durations[j] })
		f.node.P90 = percentile(f.durations, 0.9)
		best := 0
		for group, count := range f.groups {
			if count > best || (count == best && group < f.node.Group) {
				best = count
				f.node.Group = group
			}
		}
		index[prefix] = len(graph.Nodes)
		graph.Nodes = append(graph.Nodes, f.node)
	}

	// map every page to the index of its folder
	for targetURL := range nodeOf {
		nodeOf[targetURL] = index[folderAt(tree, results[targetURL].TargetURL, depth).folder.Prefix]
	}
	// count the pages linking in from outside the folder
	for _, prefix := range prefixes {
		f := folders[prefix]
		for source := range f.linked {
			if from, ok := nodeOf[source]; !ok || from != index[prefix] {
				graph.Nodes[index[prefix]].LinkedFrom++
			}
		}
	}
}

// folderAt returns the deepest folder down to depth containing the target url
func folderAt(tree *folderNode, targetURL string, depth int) *folderNode {
	origin, segments, ok := targetURLSegments(targetURL)
	if !ok {
		return nil
	}
	node := tree.children[origin]
	for level, segment := range segments {
		child, isFolder := node.children[segment]
		if level >= depth || segment == "" || !isFolder {
			break
		}
		node = child
	}
	return node
}
