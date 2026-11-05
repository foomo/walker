# htmlschema for VS Code

Completion, hover docs and live lint diagnostics for htmlschema files - the
`val:*` elements, the `val:` rules of html elements, attribute values like
`severity`, `val:include`/`val:comp` paths and attribute rule lists.

The extension starts `htmlschema-lsp` (`cmd/htmlschema-lsp`), the same language
server the Zed extension (`editors/zed-htmlschema`) uses. It runs next to VS
Code's html support and only acts on files inside a schema root - a directory
with `groups/` or `default.html` - every other html file is left alone.

## Install

1. Install the language server, from the walker repository:

   ```sh
   go install ./cmd/htmlschema-lsp
   ```

   The extension looks for it on the `PATH`, then in `$GOBIN`, `$GOPATH/bin`
   and `~/go/bin` - VS Code started from the Dock often has a `PATH` without
   them. Elsewhere set `htmlschema.serverPath` to the binary.

2. Install the extension's dependency and link it into VS Code's extensions:

   ```sh
   cd editors/vscode-htmlschema
   npm install
   ln -s "$PWD" ~/.vscode/extensions/foomo.htmlschema-0.1.0
   ```

   Then `Developer: Reload Window`. To package it instead:
   `npx @vscode/vsce package` and `code --install-extension htmlschema-0.1.0.vsix`.
   For development open this directory in VS Code and press F5.

3. Open a schema file, e.g. `htmlschema/demo/schema/groups/product.html`.

## Settings

- `htmlschema.serverPath` - the `htmlschema-lsp` binary, default `htmlschema-lsp`
- `htmlschema.trace.server` - `off`, `messages` or `verbose`, the protocol in
  the output channel

Logs: the output channel "htmlschema". After changes to the server run
`go install ./cmd/htmlschema-lsp` again and `htmlschema: Restart Language Server`.
