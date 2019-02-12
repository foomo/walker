package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/foomo/walker"
	"github.com/foomo/walker/config"
)

type server struct {
	servicePath  string
	serviveProxy http.Handler
	p            *httputil.ReverseProxy
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, s.servicePath) {
		fmt.Println("service call", r.URL.Path)
		s.serviveProxy.ServeHTTP(w, r)
		return
	}
	fmt.Println("proxying", r.URL.Path)
	s.p.ServeHTTP(w, r)
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
	spew.Dump(conf)
	proxyURL, errProxyURL := url.Parse(conf.Frontend)
	must("can not parse frontend url: "+conf.Frontend+" :", errProxyURL)
	fmt.Println("proxying frontend requests to", proxyURL)
	if conf.UseCookies {
		fmt.Println("using cookies")
		cookieJar, _ := cookiejar.New(nil)
		http.DefaultClient.Jar = cookieJar
	}

	s, errS := walker.NewService(conf)
	must("could not start service", errS)

	serviceProxy := walker.NewDefaultServiceGoTSRPCProxy(s, []string{})

	log.Fatal(http.ListenAndServe(conf.Addr, &server{
		servicePath:  serviceProxy.EndPoint,
		serviveProxy: serviceProxy,
		p:            httputil.NewSingleHostReverseProxy(proxyURL),
	}))
}
