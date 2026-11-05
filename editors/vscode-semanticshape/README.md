# Semantic Shape for VS Code

Completion, hover docs and live lint diagnostics for semantic shape files
(`*.semanticshape.html`) - the `val:*` elements, the `val:` rules of html elements,
attribute values like `severity`, `val:include`/`val:comp` paths and attribute
rule lists.

The extension starts `semanticshape-lsp` (`go/cmd/semanticshape-lsp`), the same
language server the Zed extension (`editors/zed-semanticshape`) uses. Shape files
stay html documents, so VS Code's html support keeps working next to it; the
server only acts on `*.semanticshape.html` files inside a shape root - a directory with
`groups/` or `default.semanticshape.html`.

## Install

1. Install the language server, from the walker repository:

   ```sh
   go -C go install ./cmd/semanticshape-lsp
   ```

   The extension looks for it on the `PATH`, then in `$GOBIN`, `$GOPATH/bin`
   and `~/go/bin` - VS Code started from the Dock often has a `PATH` without
   them. Elsewhere set `semanticshape.serverPath` to the binary.

2. Install the extension's dependency and link it into VS Code's extensions:

   ```sh
   cd editors/vscode-semanticshape
   npm install
   ln -s "$PWD" ~/.vscode/extensions/foomo.semanticshape-0.2.0
   ```

   Then `Developer: Reload Window`. To package it instead:
   `npx @vscode/vsce package` and `code --install-extension semanticshape-0.2.0.vsix`.
   For development open this directory in VS Code and press F5.

3. Open a shape file, e.g. `go/internal/demoshop/shapes/groups/product.semanticshape.html`.

## Settings

- `semanticshape.serverPath` - the `semanticshape-lsp` binary, default `semanticshape-lsp`
- `semanticshape.trace.server` - `off`, `messages` or `verbose`, the protocol in
  the output channel

Logs: the output channel "Semantic Shape". After changes to the server run
`go -C go install ./cmd/semanticshape-lsp` again and
`Semantic Shape: Restart Language Server`.
