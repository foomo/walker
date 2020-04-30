package example

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
)

const GroupHeader = "group"

type server struct {
	root string
}

func NewServer(root string) http.Handler {
	return &server{
		root: root,
	}
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Println("example server serving", r.URL)
	if r.URL.Path == "/" {
		w.Header().Add(GroupHeader, "content/index")
		http.ServeFile(w, r, filepath.Join(s.root, "index.html"))
		return
	}
	const routeDoc = "execting /<app>/<page>-name.html"
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) != 3 {
		http.Error(w, "app not found "+routeDoc, http.StatusNotFound)
		return
	}
	pageParts := strings.Split(pathParts[2], "-")
	page := ""
	switch len(pageParts) {
	case 1:
		page = strings.TrimSuffix(pageParts[0], ".html")
	case 2:
		page = pageParts[0]
	default:
		http.Error(w, "page not found "+routeDoc, http.StatusNotFound)
		return
	}
	group := pathParts[1] + "/" + page
	w.Header().Add(GroupHeader, group)
	http.ServeFile(w, r, filepath.Join(s.root, r.URL.Path))
}
