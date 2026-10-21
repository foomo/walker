package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"

	"github.com/foomo/walker/htmlschema"
)

func main() {
	flagHelp := flag.Bool("help", false, "show help")
	flag.Parse()

	if len(flag.Args()) != 3 || *flagHelp {
		fmt.Println("foomo walker validator - validate a html page against an html schema")
		fmt.Println("usage:", os.Args[0], "path/to/schema-root", "group", "http://server.com/doc-to-validate|path/to/file.html")
		fmt.Println("exit codes: 1 usage or schema errors, 2 page could not be loaded, 3 the page has error findings")
		os.Exit(1)
	}
	schemaRoot, group, target := flag.Arg(0), flag.Arg(1), flag.Arg(2)

	root, errLoad := htmlschema.LoadRoot(schemaRoot)
	if errLoad != nil {
		slog.Error("could not load the schema root", "root", schemaRoot)
		// the lint errors, one per line - file:line message
		fmt.Fprintln(os.Stderr, errLoad)
		os.Exit(1)
	}

	htmlBytes, errRead := read(target)
	if errRead != nil {
		slog.Error("could not load the page", "target", target, "error", errRead)
		os.Exit(2)
	}

	report, errValidate := root.Validate(group, htmlBytes)
	if errValidate != nil {
		slog.Error("could not validate", "target", target, "error", errValidate)
		os.Exit(2)
	}
	report.Print(os.Stdout)
	if report.Count(htmlschema.SeverityError) > 0 {
		os.Exit(3)
	}
}

// read loads an http(s) url or a local file
func read(target string) ([]byte, error) {
	u, errParse := url.Parse(target)
	if errParse != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return os.ReadFile(target)
	}
	resp, errGet := http.Get(target)
	if errGet != nil {
		return nil, errGet
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("unexpected status: " + resp.Status)
	}
	return io.ReadAll(resp.Body)
}
