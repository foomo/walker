package walker

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"

	"github.com/PuerkitoBio/goquery"
	"github.com/foomo/walker/go/internal/config"
	"github.com/foomo/walker/go/internal/htmlschema"
	"github.com/foomo/walker/go/internal/reports"
	"github.com/foomo/walker/go/internal/vo"
)

type start struct {
	conf                     config.Config
	schemaRoot               *htmlschema.Root
	linkListFilterFunc       LinkListFilterFunc
	validationFunc           ValidationFunc
	scrapeFunc               ScrapeFunc
	scrapeResultModifierFunc ScrapeResultModifierFunc
}

type statuses struct {
	complete *vo.Status
	running  *vo.Status
}

type started struct {
	Err              error
	ChanLoopComplete chan vo.Status
}

type sortLenStrings []string

func (p sortLenStrings) Len() int           { return len(p) }
func (p sortLenStrings) Less(i, j int) bool { return len(p[i]) > len(p[j]) }
func (p sortLenStrings) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func sortPathsByLength(paths []string) []string {
	sls := make(sortLenStrings, len(paths))
	copy(sls, paths)
	sort.Sort(sls)
	return []string(sls)
}

type LinkListFilterFunc func(baseURL, docURL *url.URL, doc *goquery.Document) (ll vo.LinkList, err error)
type ScrapeFunc func(response *http.Response) (scarepeData interface{}, err error)
type ScrapeResultModifierFunc func(result vo.ScrapeResult) (modifiedResult vo.ScrapeResult, err error)
type ValidationFunc func(structure vo.Structure, scrapeData interface{}) (vo.Validations, error)

type Walker struct {
	chanResult   chan scrapeResultAndClient
	chanStart    chan start
	chanStatus   chan vo.Status
	chanStop     chan vo.Status
	chanStarted  chan started
	chanStatuses chan statuses
	chanRestart  chan bool
	// closed, when the scrape loop has stopped
	chanDone chan struct{}
	metrics  *metrics
}

func NewWalker() *Walker {
	w := &Walker{
		chanResult:   make(chan scrapeResultAndClient),
		chanStart:    make(chan start),
		chanStop:     make(chan vo.Status),
		chanStatus:   make(chan vo.Status),
		chanStarted:  make(chan started),
		chanStatuses: make(chan statuses),
		chanRestart:  make(chan bool),
		chanDone:     make(chan struct{}),
	}
	w.metrics = newMetrics(w.GetStatuses)
	go w.scrapeloop()
	return w
}

// Walk starts walking - the returned channel receives the status of every
// completed scrape cycle. Receiving is optional: the channel only buffers the
// latest completed cycle, older ones are dropped, if they were not received.
func (w *Walker) Walk(
	conf *config.Config,
	linkListFilter LinkListFilterFunc,
	scrapeFunc ScrapeFunc,
	validationFunc ValidationFunc,
	scrapeResultModifierFunc ScrapeResultModifierFunc,
) (chanLoopStatus chan vo.Status, err error) {
	var schemaRoot *htmlschema.Root
	if conf.SchemaRoot != "" {
		root, errLoad := htmlschema.LoadRoot(conf.SchemaRoot)
		if errLoad != nil {
			return nil, errLoad
		}
		schemaRoot = root
	}
	w.chanStart <- start{
		schemaRoot:               schemaRoot,
		conf:                     *conf,
		scrapeFunc:               scrapeFunc,
		linkListFilterFunc:       linkListFilter,
		validationFunc:           validationFunc,
		scrapeResultModifierFunc: scrapeResultModifierFunc,
	}
	st := <-w.chanStarted
	return st.ChanLoopComplete, st.Err
}

func (w *Walker) Stop() vo.Status {
	w.chanStop <- vo.Status{}
	return <-w.chanStop
}

// Restart starts a new scrape cycle right away - a running cycle is
// aborted, the wait for a scheduled one ends. False, if there is no walk to
// restart.
func (w *Walker) Restart() bool {
	select {
	case w.chanRestart <- false:
	case <-w.chanDone:
		return false
	}
	return <-w.chanRestart
}

// GetStatus returns the status of the current cycle - once a cycle has
// completed and walker waits for the next one, it is that completed cycle
// with Running == false
func (w *Walker) GetStatus() vo.Status {
	w.chanStatus <- vo.Status{}
	return <-w.chanStatus
}

// GetStatuses returns the last completed cycle and the cycle in progress,
// either one is nil if there is none - both are nil, once the walker was stopped
func (w *Walker) GetStatuses() (complete, running *vo.Status) {
	select {
	case w.chanStatuses <- statuses{}:
	case <-w.chanDone:
		return nil, nil
	}
	st := <-w.chanStatuses
	return st.complete, st.running
}

func line(w io.Writer) {
	fmt.Fprintln(w, "------------------------------------------------------------------------")
}

func headline(w io.Writer, v ...interface{}) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, v...)
	line(w)
}

func (wlkr *Walker) GetReportHandler(basePath string) http.HandlerFunc {
	h := reports.GetReportHandler(basePath)
	return func(w http.ResponseWriter, r *http.Request) {
		completeStatus, runningStatus := wlkr.GetStatuses()
		h(w, r, completeStatus, runningStatus)
	}
}
