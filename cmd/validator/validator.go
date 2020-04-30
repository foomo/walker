package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"

	"github.com/foomo/walker/htmlschema"
)

func main() {
	flagHelp := flag.Bool("help", false, "show help")
	flag.Parse()

	if len(flag.Args()) != 2 || *flagHelp {
		fmt.Println("foomo walker validator - validate a html page against an html validaation schema")
		fmt.Println("usage", os.Args[0], "path/to/schema.html", "http://server.com/doc-to-validate")
		os.Exit(1)
	}

	schemaFile := flag.Arg(0)
	urlToValidate := flag.Arg(1)

	u, errParse := url.Parse(urlToValidate)
	if errParse != nil {
		fmt.Println("can not parse url to validate", errParse)
		os.Exit(1)
	}

	if u.Scheme == "" {
		urlToValidate = "file://" + urlToValidate
	}

	fmt.Println("loading schema", schemaFile)

	schema, errLoad := htmlschema.Load(schemaFile)
	if errLoad != nil {
		fmt.Println("could not load scheme", errLoad)
		os.Exit(1)
	}
	fmt.Println("validating url", urlToValidate)
	report, errValidate := schema.ValidateURL(
		urlToValidate,
		os.Stdout,
	)
	if errValidate != nil {
		fmt.Println("could not validate", errValidate)
		os.Exit(2)
	}
	report.Print(os.Stdout)
}
