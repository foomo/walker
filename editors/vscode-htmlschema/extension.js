// htmlschema for VS Code: starts htmlschema-lsp (cmd/htmlschema-lsp of the
// walker repository) for html files. The server only acts on files inside a
// schema root - a directory with groups/ or default.html - and leaves every
// other html file alone, so it runs next to VS Code's own html support.
const fs = require("fs");
const os = require("os");
const path = require("path");
const vscode = require("vscode");
const { LanguageClient, TransportKind } = require("vscode-languageclient/node");

const installHint = "install it from the walker repository with: go install ./cmd/htmlschema-lsp";

/** @type {LanguageClient | undefined} */
let client;
/** @type {vscode.OutputChannel | undefined} */
let output;

function isExecutable(file) {
  try {
    fs.accessSync(file, fs.constants.X_OK);
    return fs.statSync(file).isFile();
  } catch {
    return false;
  }
}

// resolveServer finds the binary: an explicit path as it is, a bare name on the
// PATH and in go's bin directories - VS Code started from the Dock often has a
// PATH without them
function resolveServer(setting) {
  const name = setting || "htmlschema-lsp";
  if (name.includes("/") || name.includes("\\")) {
    const file = name.startsWith("~") ? path.join(os.homedir(), name.slice(1)) : name;
    return isExecutable(file) ? file : undefined;
  }
  const exe = process.platform === "win32" && !name.endsWith(".exe") ? name + ".exe" : name;
  const dirs = (process.env.PATH || "").split(path.delimiter).filter(Boolean);
  if (process.env.GOBIN) dirs.push(process.env.GOBIN);
  for (const gopath of (process.env.GOPATH || "").split(path.delimiter).filter(Boolean)) {
    dirs.push(path.join(gopath, "bin"));
  }
  dirs.push(path.join(os.homedir(), "go", "bin"));
  return dirs.map((dir) => path.join(dir, exe)).find(isExecutable);
}

async function start() {
  const setting = vscode.workspace.getConfiguration("htmlschema").get("serverPath");
  const command = resolveServer(setting);
  if (!command) {
    vscode.window.showErrorMessage(`htmlschema: ${setting || "htmlschema-lsp"} not found - ${installHint}`);
    return;
  }
  output.appendLine(`starting ${command}`);
  client = new LanguageClient(
    "htmlschema",
    "htmlschema",
    { command, transport: TransportKind.stdio },
    {
      documentSelector: [{ scheme: "file", language: "html" }],
      outputChannel: output,
    },
  );
  try {
    await client.start();
  } catch (err) {
    vscode.window.showErrorMessage(`htmlschema: the language server did not start: ${err.message || err}`);
  }
}

async function stop() {
  const running = client;
  client = undefined;
  if (running) {
    await running.stop();
  }
}

async function restart() {
  await stop();
  await start();
}

/** @param {vscode.ExtensionContext} context */
async function activate(context) {
  output = vscode.window.createOutputChannel("htmlschema");
  context.subscriptions.push(
    output,
    vscode.commands.registerCommand("htmlschema.restartServer", restart),
    vscode.workspace.onDidChangeConfiguration((event) => {
      if (event.affectsConfiguration("htmlschema.serverPath")) {
        restart();
      }
    }),
  );
  await start();
}

function deactivate() {
  return stop();
}

module.exports = { activate, deactivate };
