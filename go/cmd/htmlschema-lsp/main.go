// htmlschema-lsp is a language server for htmlschema files: completion of
// val:* elements, rules and values, hover docs, and the lint errors of the
// schema root as diagnostics while typing. It speaks lsp over stdio.
package main

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		fmt.Println("htmlschema-lsp - language server for htmlschema files, speaks lsp over stdio")
		return
	}
	s := newServer(&conn{in: bufio.NewReader(os.Stdin), out: os.Stdout})
	if err := s.run(); err != nil && err != io.EOF {
		// stdout is the protocol - slog writes to stderr, the log of the editor
		slog.Error("htmlschema-lsp stopped", "error", err)
		os.Exit(1)
	}
}
