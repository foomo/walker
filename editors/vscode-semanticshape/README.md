# Semantic Shape for VS Code

Completion, hover docs and live lint diagnostics for semantic shape files
(`*.semanticshape.html`) - the `val:*` elements, the `val:` rules of html elements,
attribute values like `severity`, `val:include`/`val:comp` paths and attribute
rule lists.

The extension starts its bundled `semanticshape-lsp` (`go/cmd/semanticshape-lsp`),
the same language server the Zed extension (`editors/zed-semanticshape`) uses. Shape files
stay html documents, so VS Code's html support keeps working next to it; the
server only acts on `*.semanticshape.html` files inside a shape root - a directory with
`groups/` or `default.semanticshape.html`.

## Install

The extension brings its own `semanticshape-lsp` for macOS (arm64, x64),
Linux (x64, arm64) and Windows (x64), in `server/<platform>-<arch>`. From the
walker repository:

```sh
mise run editors:vscode:install   # build the servers, package the .vsix and install it
```

`mise run editors:vscode` only packages
`editors/vscode-semanticshape/semanticshape-<version>.vsix` - install it with
`code --install-extension semanticshape-0.2.0.vsix`.

For development: `mise run editors:vscode:server` builds the servers, then
`npm install` in this directory and `ln -s "$PWD" ~/.vscode/extensions/foomo.semanticshape-0.2.0`,
or open the directory in VS Code and press F5. Without `server/` the
extension looks for `semanticshape-lsp` on the `PATH`, in `$GOBIN`,
`$GOPATH/bin` and `~/go/bin` (`go -C go install ./cmd/semanticshape-lsp`).

Open a shape file, e.g. `go/internal/demoshop/shapes/groups/product.semanticshape.html`.

## Settings

- `semanticshape.serverPath` - a `semanticshape-lsp` binary to use instead of the bundled one, e.g. one you are working on; empty by default
- `semanticshape.trace.server` - `off`, `messages` or `verbose`, the protocol in
  the output channel

Logs: the output channel "Semantic Shape" - it names the binary it started.
After changes to the server run `mise run editors:vscode:install` again, or
point `semanticshape.serverPath` at a binary you build with
`go -C go build -o /tmp/semanticshape-lsp ./cmd/semanticshape-lsp` and run
`Semantic Shape: Restart Language Server`.
