package walker

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/PuerkitoBio/goquery"
	"github.com/foomo/walker/config"
	"github.com/foomo/walker/vo"
)

type start struct {
	conf               config.Config
	linkListFilterFunc LinkListFilterFunc
	scrapeFunc         ScrapeFunc
}

type started struct {
	Err              error
	ChanLoopComplete chan vo.Status
}

type LinkListFilterFunc func(baseURL, docURL *url.URL, doc *goquery.Document) (ll vo.LinkList, err error)
type ScrapeFunc func(response *http.Response) error

type Walker struct {
	chanResult     chan scrapeResultAndClient
	chanStart      chan start
	chanStatus     chan vo.Status
	chanStarted    chan started
	CompleteStatus *vo.Status
}

func NewWalker() *Walker {
	w := &Walker{
		chanResult:  make(chan scrapeResultAndClient),
		chanStart:   make(chan start),
		chanStatus:  make(chan vo.Status),
		chanStarted: make(chan started),
	}
	go w.scrapeloop()
	return w
}

func (w *Walker) Walk(
	conf *config.Config,
	linkListFilter LinkListFilterFunc,
	scrapeFunc ScrapeFunc,
) (chanLoopStatus chan vo.Status, err error) {
	w.chanStart <- start{
		conf:               *conf,
		scrapeFunc:         scrapeFunc,
		linkListFilterFunc: linkListFilter,
	}
	st := <-w.chanStarted
	return st.ChanLoopComplete, st.Err
}

func (w *Walker) GetStatus() vo.Status {
	w.chanStatus <- vo.Status{}
	return <-w.chanStatus
}

func line(w io.Writer) {
	fmt.Fprintln(w, "------------------------------------------------------------------------")
}

func headline(w io.Writer, v ...interface{}) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, v...)
	line(w)
}
