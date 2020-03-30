package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/foomo/walker"
	"github.com/foomo/walker/config"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	yaml "gopkg.in/yaml.v2"
)

type server struct {
	s              *walker.Service
	conf           string
	reportHandler  http.HandlerFunc
	metricsHandler http.HandlerFunc
}

const pathReports = "/reports"

const htmlIndex = `<html>
<head><title>Walker</title></head>
<body>
	<h1>Walker</h1>
	<ul>
		<li><a href="/status">crawling status</a></li>
		<li><a href="/metrics">prometheus metrics scraping endpoint</a></li>
		<li><a href="/reports/summary">summary of status codes and performance overview</a></li>
		<li><a href="/reports/results">all plain results (this can be a very long doc)</a></li>
		<li><a href="/reports/list">list of all jobs / results</a></li>
		<li><a href="/reports/highscore">highscore - all results sorted by request duration</a></li>
		<li><a href="/reports/broken-links">broken links</a></li>
		<li><a href="/reports/seo">seo</a></li>
		<li><a href="/reports/errors">errors - calls that returned error status codes</a></li>
	</ul>
</body>
</html>`

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		w.Write([]byte(htmlIndex))
		return
	}
	if r.URL.Path == "/metrics" {
		s.metricsHandler(w, r)
		return
	}
	if r.URL.Path == "/status" {
		w.Write([]byte(":::::::::::::::::: STATUS ::::::::::::::::::\n"))
		w.Write([]byte("\nrunning with config:\n\n" + s.conf + "\n"))
		s.s.Walker.PrintStatus(w, s.s.Walker.GetStatus())
		return
	}
	if strings.HasPrefix(r.URL.Path, pathReports) {
		s.reportHandler(w, r)
		return
	}
	http.NotFound(w, r)
}

func must(comment string, err error) {
	if err != nil {
		fmt.Println(comment, err)
		os.Exit(1)
	}
}

func main() {
	flag.Parse()
	if len(flag.Args()) != 1 {
		fmt.Println("usage:", os.Args[0], "path/to/config.yaml")
		os.Exit(1)
	}
	conf, errConf := config.Get(flag.Arg(0))
	must("config error:", errConf)

	yamlConfBytes, _ := yaml.Marshal(conf)
	fmt.Println("this is how I understood your config:")
	fmt.Println("------------------------------------------------------------------")
	fmt.Println(string(yamlConfBytes))
	fmt.Println("------------------------------------------------------------------")

	s, chanLoopComplete, errS := walker.NewService(conf, nil, nil)

	must("could not start service", errS)

	go func() {
		for {
			select {
			case completeStatus := <-chanLoopComplete:
				fmt.Println("a loop was completed, I walked around", len(completeStatus.Results), "docs")
			}
		}
	}()

	log.Fatal(http.ListenAndServe(conf.Addr, &server{
		conf:           string(yamlConfBytes),
		s:              s,
		reportHandler:  walker.GetReportHandler(pathReports, s.Walker),
		metricsHandler: promhttp.Handler().ServeHTTP,
	}))
}
