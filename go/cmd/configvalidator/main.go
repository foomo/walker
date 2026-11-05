// configvalidator checks walker config files - and is the yaml backend of
// the config editor of the desktop app, which bundles it as a helper:
//
//	configvalidator [-json] [-base dir] config.yaml|-      validate
//	configvalidator read [-base dir] config.yaml|-          form and problems as json
//	configvalidator write config.yaml < form.json           the config with the form applied
//	configvalidator write - < {"yaml": "…", "form": {…}}    the same, all on stdin
//
// - reads stdin. Exit codes: 0 valid (warnings are fine), 1 usage or io, 3 errors.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/foomo/walker/go/internal/config"
)

func main() {
	args := os.Args[1:]
	command := "validate"
	if len(args) > 0 && (args[0] == "read" || args[0] == "write" || args[0] == "validate") {
		command, args = args[0], args[1:]
	}
	flags := flag.NewFlagSet("configvalidator", flag.ExitOnError)
	asJSON := flags.Bool("json", false, "print the problems as json")
	base := flags.String("base", "", "directory relative paths in the config are resolved against - default: the directory of the config file")
	flags.Usage = usage
	_ = flags.Parse(args)
	if flags.NArg() != 1 {
		usage()
		os.Exit(1)
	}
	file := flags.Arg(0)
	if *base == "" {
		*base = baseOf(file)
	}

	var err error
	exit := 0
	switch command {
	case "validate":
		exit, err = validate(file, *base, *asJSON)
	case "read":
		err = read(file, *base)
	case "write":
		err = write(file)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "configvalidator:", err)
		os.Exit(1)
	}
	os.Exit(exit)
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  configvalidator [-json] [-base dir] config.yaml|-
  configvalidator read [-base dir] config.yaml|-
  configvalidator write config.yaml|- < form.json`)
}

func baseOf(file string) string {
	if file == "-" {
		dir, _ := os.Getwd()
		return dir
	}
	dir, _ := filepath.Abs(filepath.Dir(file))
	return dir
}

func input(file string) ([]byte, error) {
	if file == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(file)
}

func validate(file, base string, asJSON bool) (int, error) {
	data, err := input(file)
	if err != nil {
		return 0, err
	}
	problems := config.Validate(data, base)
	exit := 0
	for _, p := range problems {
		if p.Severity == config.SeverityError {
			exit = 3
		}
	}
	if asJSON {
		return exit, json.NewEncoder(os.Stdout).Encode(map[string]any{"problems": nonNil(problems)})
	}
	name := file
	if file == "-" {
		name = "stdin"
	}
	for _, p := range problems {
		fmt.Printf("%s:%d:%d %s %s\n", name, p.Line, p.Column, p.Severity, p.Message)
	}
	if len(problems) == 0 {
		fmt.Println(name + ": ok")
	}
	return exit, nil
}

func read(file, base string) error {
	data, err := input(file)
	if err != nil {
		return err
	}
	form, err := config.ReadForm(data)
	if err != nil {
		form = config.DefaultForm()
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"form":     form,
		"problems": nonNil(config.Validate(data, base)),
	})
}

func write(file string) error {
	var request struct {
		YAML string      `json:"yaml"`
		Form config.Form `json:"form"`
	}
	var data []byte
	if file == "-" {
		if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
			return err
		}
		data = []byte(request.YAML)
	} else {
		if err := json.NewDecoder(os.Stdin).Decode(&request.Form); err != nil {
			return err
		}
		var err error
		if data, err = os.ReadFile(file); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	written, err := config.WriteForm(data, request.Form)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(written)
	return err
}

func nonNil(problems []config.Problem) []config.Problem {
	if problems == nil {
		return []config.Problem{}
	}
	return problems
}
