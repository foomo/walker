# Semantic Shape for Zed

Completion, hover docs and live lint diagnostics for semantic shape files
(`*.semanticshape.html`) - the `val:*` elements, the `val:` rules of html elements,
attribute values like `severity`, `val:include`/`val:comp` paths and attribute
rule lists.

The extension starts `semanticshape-lsp` (`go/cmd/semanticshape-lsp`). It runs
next to Zed's html language server and only acts on `*.semanticshape.html` files inside
a shape root - a directory with `groups/` or `default.semanticshape.html` - every other
html file is left alone.

## Install

1. Install the language server on your PATH, from the walker repository:

   ```sh
   go -C go install ./cmd/semanticshape-lsp
   ```

2. In Zed: `zed: install dev extension` and choose this directory
   (`editors/zed-semanticshape`). Zed builds it with your rustup toolchain.
   If the old `htmlschema` dev extension is still installed, uninstall it.

3. Open a shape file, e.g. `go/internal/demoshop/shapes/groups/product.semanticshape.html`.

If the server does not start for html files, enable it explicitly in the
settings:

```json
{
  "languages": {
    "HTML": {
      "language_servers": ["semanticshape-lsp", "..."]
    }
  }
}
```

Logs: `zed: open log`, or `debug: open language server logs`.
After changes to the server run `go -C go install ./cmd/semanticshape-lsp`
again and `editor: restart language server`.
