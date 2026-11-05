package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/foomo/walker/go/internal/config"
	"github.com/foomo/walker/go/internal/crawler"
	"github.com/foomo/walker/go/internal/metrics"
	"github.com/foomo/walker/go/internal/reports"
	"github.com/foomo/walker/go/internal/service"
	"github.com/foomo/walker/go/internal/vo"
	yaml "gopkg.in/yaml.v3"
)

type server struct {
	serviceProxy  *service.ServiceGoTSRPCProxy
	walker        *crawler.Walker
	conf          string
	reportHandler func(
		w http.ResponseWriter, r *http.Request,
		completeStatus, runningStatus *vo.Status,
	)
	metricsHandler http.HandlerFunc
}

const (
	pathReports = "/reports"
	pathService = "/service"
)

func htmlIndex() []byte {
	return []byte(
		`<html>
			<head><title>Walker</title></head>
			<body>
			<style>
				body {
					color: black;
					font-family: 'Courier New', Courier, monospace;

				}
				table {
					border-collapse: collapse;
					border: 1px solid black;
					width: 100%;
				}
				table tr, table tr td {
					padding: 0.5rem;
					border: 1px solid black;
				}
			</style>
			<h1>walker</h1>
			<ul>
				<li><a href="/status">crawling status</a></li>
				<li><a href="/metrics">prometheus metrics scraping endpoint</a></li>
			</ul>

			` + reports.GetReportHandlerMenuHTML(pathReports) + `
			</body>
		</html>`)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		w.Write(htmlIndex())
		return
	}
	if r.URL.Path == "/metrics" {
		s.metricsHandler(w, r)
		return
	}
	if r.URL.Path == "/status" {
		w.Write([]byte(":::::::::::::::::: STATUS ::::::::::::::::::\n"))
		w.Write([]byte("\nrunning with config:\n\n" + s.conf + "\n"))
		reports.PrintStatus(w, s.walker.GetStatus())
		return
	}
	switch {
	case strings.HasPrefix(r.URL.Path, pathReports):
		completeStatus, runningStatus := s.walker.GetStatuses()
		s.reportHandler(w, r, completeStatus, runningStatus)
	case strings.HasPrefix(r.URL.Path, pathService+"/"):
		s.serviceProxy.ServeHTTP(w, r)
	default:
		http.NotFound(w, r)
	}
}

func must(msg string, err error) {
	if err != nil {
		slog.Error(msg, "error", err)
		os.Exit(1)
	}
}

// newLogger is the slog logger of walker - text for people, the macOS app
// shows it in its log view, json for log collectors
func newLogger(level, format string) (*slog.Logger, error) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("unknown log level %q - debug, info, warn or error", level)
	}
	opts := &slog.HandlerOptions{Level: l}
	switch format {
	case "text":
		return slog.New(slog.NewTextHandler(os.Stderr, opts)), nil
	case "json":
		return slog.New(slog.NewJSONHandler(os.Stderr, opts)), nil
	}
	return nil, fmt.Errorf("unknown log format %q - text or json", format)
}

func main() {
	exitOnStdinClose := flag.Bool("exit-on-stdin-close", false, "exit when stdin is closed - the macOS app holds it open, so walker never outlives the app")
	logLevel := flag.String("log-level", "info", "debug, info, warn or error")
	logFormat := flag.String("log-format", "text", "text or json")
	flag.Parse()
	if len(flag.Args()) != 1 {
		fmt.Fprintln(os.Stderr, "usage:", os.Args[0], "[-exit-on-stdin-close] [-log-level info] [-log-format text] path/to/config.yaml")
		os.Exit(1)
	}
	logger, errLogger := newLogger(*logLevel, *logFormat)
	if errLogger != nil {
		fmt.Fprintln(os.Stderr, errLogger)
		os.Exit(1)
	}
	slog.SetDefault(logger)
	if *exitOnStdinClose {
		go func() {
			_, _ = io.Copy(io.Discard, os.Stdin)
			slog.Info("stdin closed, exiting")
			os.Exit(0)
		}()
	}
	conf, errConf := config.Get(flag.Arg(0))
	must("could not load the config", errConf)
	yamlConfBytes, _ := yaml.Marshal(conf)
	slog.Info("starting walker",
		"config", flag.Arg(0),
		"addr", conf.Addr,
		"target", conf.Target.BaseURL,
		"paths", conf.Target.Paths,
		"concurrency", conf.Concurrency,
		"interval", conf.Interval,
		"schemaroot", conf.SchemaRoot,
	)
	slog.Debug("config", "yaml", string(yamlConfBytes))

	m := metrics.New()
	w := crawler.New(m)
	m.Collect(w.GetStatuses)
	// no custom link filter, scrape, validation or result modifier funcs
	chanLoopComplete, errWalk := w.Walk(conf, nil, nil, nil, nil)
	must("could not start walking", errWalk)
	// the scrape loop logs its cycles, completed statuses are not needed here
	go func() {
		for range chanLoopComplete {
		}
	}()

	err := http.ListenAndServe(conf.Addr, &server{
		conf:           string(yamlConfBytes),
		serviceProxy:   service.NewDefaultServiceGoTSRPCProxy(service.New(w, conf)),
		walker:         w,
		reportHandler:  reports.GetReportHandler(pathReports),
		metricsHandler: m.Handler().ServeHTTP,
	})
	must("could not serve on "+conf.Addr, err)
}
