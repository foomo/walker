// semanticshape-lsp is a language server for semantic shape files: completion of
// val:* elements, rules and values, hover docs, and the lint errors of the
// shape root as diagnostics while typing. It speaks lsp over stdio.
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
		fmt.Println("semanticshape-lsp - language server for semantic shape files, speaks lsp over stdio")
		return
	}
	s := newServer(&conn{in: bufio.NewReader(os.Stdin), out: os.Stdout})
	if err := s.run(); err != nil && err != io.EOF {
		// stdout is the protocol - slog writes to stderr, the log of the editor
		slog.Error("semanticshape-lsp stopped", "error", err)
		os.Exit(1)
	}
}
