# htmlschema for Zed

Completion, hover docs and live lint diagnostics for htmlschema files - the
`val:*` elements, the `val:` rules of html elements, attribute values like
`severity`, `val:include`/`val:comp` paths and attribute rule lists.

The extension starts `htmlschema-lsp` (`go/cmd/htmlschema-lsp`). It runs next to
Zed's html language server and only acts on files inside a schema root - a
directory with `groups/` or `default.html` - every other html file is left alone.

## Install

1. Install the language server on your PATH, from the walker repository:

   ```sh
   go -C go install ./cmd/htmlschema-lsp
   ```

2. In Zed: `zed: install dev extension` and choose this directory
   (`editors/zed-htmlschema`). Zed builds it with your rustup toolchain.

3. Open a schema file, e.g. `go/internal/demoshop/schema/groups/product.html`.

If the server does not start for html files, enable it explicitly in the
settings:

```json
{
  "languages": {
    "HTML": {
      "language_servers": ["htmlschema-lsp", "..."]
    }
  }
}
```

Logs: `zed: open log`, or `debug: open language server logs`.
After changes to the server run `go -C go install ./cmd/htmlschema-lsp` again and
`editor: restart language server`.
