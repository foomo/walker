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
	"github.com/foomo/walker/reports"
	"github.com/foomo/walker/vo"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	yaml "gopkg.in/yaml.v3"
)

type server struct {
	s             *walker.Service
	conf          string
	reportHandler func(
		w http.ResponseWriter, r *http.Request,
		completeStatus, runningStatus *vo.Status,
	)
	metricsHandler http.HandlerFunc
}

const pathReports = "/reports"

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
		s.s.Walker.PrintStatus(w, s.s.Walker.GetStatus())
		return
	}
	if strings.HasPrefix(r.URL.Path, pathReports) {
		runningStatus := s.s.Walker.GetStatus()
		s.reportHandler(w, r, s.s.Walker.CompleteStatus, &runningStatus)
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

	s, chanLoopComplete, errS := walker.NewService(conf, nil, nil, nil)

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
		reportHandler:  reports.GetReportHandler(pathReports),
		metricsHandler: promhttp.Handler().ServeHTTP,
	}))
}
