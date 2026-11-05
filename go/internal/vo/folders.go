package vo

import (
	"sort"
	"strings"
)

// Folder is a node of the path tree of the scraped urls - only path segments
// with pages below them are folders, pages themselves are not part of the tree
type Folder struct {
	Name string
	// Prefix is the url of the folder without a trailing slash, e.g.
	// https://example.com/shop - see InFolder for what it contains
	Prefix string
	// Pages is the number of pages in the folder and its sub folders - the
	// folder's own page included
	Pages int
	// Errors is the number of pages with a status >= 400 or a failed request
	Errors  int
	Folders []Folder
}

type folderNode struct {
	folder   Folder
	children map[string]*folderNode
}

// child returns the child for key - roots are keyed by origin, folders by segment
func (n *folderNode) child(key, name, prefix string) *folderNode {
	if n.children == nil {
		n.children = map[string]*folderNode{}
	}
	c, ok := n.children[key]
	if !ok {
		c = &folderNode{folder: Folder{Name: name, Prefix: prefix}}
		n.children[key] = c
	}
	return c
}

func (n *folderNode) count(r ScrapeResult) {
	n.folder.Pages++
	if IsBroken(r.Code) {
		n.folder.Errors++
	}
}

// folders converts the children, sorted by name - leaves stay nil
func (n *folderNode) folders() []Folder {
	if len(n.children) == 0 {
		return nil
	}
	folders := make([]Folder, 0, len(n.children))
	for _, c := range n.children {
		f := c.folder
		f.Folders = c.folders()
		folders = append(folders, f)
	}
	sort.Slice(folders, func(i, j int) bool { return folders[i].Name < folders[j].Name })
	return folders
}

// Folders builds the folder tree of the results with one root per scheme and
// host. It works on the raw target urls, so that a folder counts exactly the
// target urls InFolder(targetURL, folder.Prefix) matches.
func Folders(results map[string]ScrapeResult) []Folder {
	root := folderTree(results)
	for _, r := range results {
		origin, segments, ok := targetURLSegments(r.TargetURL)
		if !ok {
			continue
		}
		node := root.children[origin]
		node.count(r)
		for _, segment := range segments {
			child, isFolder := node.children[segment]
			if segment == "" || !isFolder {
				break
			}
			node = child
			node.count(r)
		}
	}
	return root.folders()
}

// folderTree builds the empty folder tree - a segment is a folder, if there
// are pages below it - so that a folder's own page counts, whatever the order
// of the results
func folderTree(results map[string]ScrapeResult) *folderNode {
	root := &folderNode{}
	for _, r := range results {
		origin, segments, ok := targetURLSegments(r.TargetURL)
		if !ok {
			continue
		}
		node := root.child(origin, displayHost(origin), origin)
		prefix := origin
		// the last segment is the page itself - after a trailing slash it is empty
		for _, segment := range segments[:len(segments)-1] {
			prefix += "/" + segment
			node = node.child(segment, segment, prefix)
		}
	}
	return root
}

// InFolder reports, if a target url is the folder itself or below it:
// https://example.com/shop contains https://example.com/shop, .../shop/,
// .../shop/a and .../shop?page=2 - but not .../shopping
func InFolder(targetURL, folder string) bool {
	if !strings.HasPrefix(targetURL, folder) {
		return false
	}
	rest := targetURL[len(folder):]
	return rest == "" || strings.ContainsRune("/?#", rune(rest[0]))
}

// targetURLSegments splits "https://host/a/b?c" into "https://host" and [a b]
func targetURLSegments(targetURL string) (origin string, segments []string, ok bool) {
	origin, path, ok := splitTargetURL(targetURL)
	if !ok {
		return "", nil, false
	}
	return origin, strings.Split(strings.TrimPrefix(path, "/"), "/"), true
}

// URLPath returns the path of a target url - without scheme, host, query
// and fragment - e.g. "/a/b" for "https://host/a/b?c"
func URLPath(targetURL string) string {
	_, path, _ := splitTargetURL(targetURL)
	return path
}

// splitTargetURL splits "https://host/a/b?c" into "https://host" and "/a/b"
func splitTargetURL(targetURL string) (origin, path string, ok bool) {
	schemeEnd := strings.Index(targetURL, "://")
	if schemeEnd < 0 {
		return "", "", false
	}
	hostStart := schemeEnd + 3
	rest := targetURL[hostStart:]
	pathStart := strings.IndexAny(rest, "/?#")
	if pathStart < 0 {
		return targetURL, "", true
	}
	origin = targetURL[:hostStart+pathStart]
	path = rest[pathStart:]
	if end := strings.IndexAny(path, "?#"); end >= 0 {
		path = path[:end]
	}
	return origin, path, true
}

// displayHost strips the scheme and credentials
func displayHost(origin string) string {
	host := origin[strings.Index(origin, "://")+3:]
	if at := strings.LastIndex(host, "@"); at >= 0 {
		host = host[at+1:]
	}
	return host
}
