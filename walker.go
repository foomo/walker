package walker

import (
	"fmt"
	"io"

	"github.com/foomo/walker/config"
)

type start struct {
	conf config.Config
}

type Status struct {
	Results              map[string]ScrapeResult
	Jobs                 map[string]bool
	ScrapeSpeed          float64
	ScrapeSpeedAverage   float64
	ScrapeWindowRequests int64
	ScrapeWindowSeconds  int64
	ScrapeTotalRequests  int64
	ScrapeTotalSeconds   int64
}

type Walker struct {
	chanResult     chan ScrapeResult
	chanStart      chan start
	chanStatus     chan Status
	chanErrStart   chan error
	CompleteStatus *Status
}

func NewWalker() *Walker {
	w := &Walker{
		chanResult:   make(chan ScrapeResult),
		chanStart:    make(chan start),
		chanStatus:   make(chan Status),
		chanErrStart: make(chan error),
	}
	go w.scrapeloop()
	return w
}

func (w *Walker) walk(conf *config.Config) error {
	w.chanStart <- start{
		conf: *conf,
	}
	return <-w.chanErrStart
}

func (w *Walker) GetStatus() Status {
	w.chanStatus <- Status{}
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
