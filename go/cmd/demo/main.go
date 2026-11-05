// demo runs the htmlschema demo shop - funny robots and unusual cars with
// great, and a few deliberately flawed, SEO
package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/foomo/walker/go/internal/htmlschema/demo/server"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	embedded := flag.Bool("embedded", false, "serve the files compiled into the binary, even if the sources are at hand")
	flag.Parse()
	slog.Info("htmlschema demo shop", "addr", *addr)
	if err := server.Run(*addr, *embedded); err != nil {
		slog.Error("demo shop stopped", "error", err)
		os.Exit(1)
	}
}
