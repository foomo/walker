// Semantic Shape for VS Code: starts semanticshape-lsp (go/cmd/semanticshape-lsp
// of the walker repository) for semantic shape files, *.shape.html. They stay
// html documents, so VS Code's own html support keeps working next to it; the
// server only acts on shape files inside a schema root - a directory with
// groups/ or default.shape.html.
const fs = require("fs");
const os = require("os");
const path = require("path");
const vscode = require("vscode");
const { LanguageClient, TransportKind } = require("vscode-languageclient/node");

const installHint = "install it from the walker repository with: go -C go install ./cmd/semanticshape-lsp";

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
  const name = setting || "semanticshape-lsp";
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
  const setting = vscode.workspace.getConfiguration("semanticshape").get("serverPath");
  const command = resolveServer(setting);
  if (!command) {
    vscode.window.showErrorMessage(`Semantic Shape: ${setting || "semanticshape-lsp"} not found - ${installHint}`);
    return;
  }
  output.appendLine(`starting ${command}`);
  client = new LanguageClient(
    "semanticshape",
    "Semantic Shape",
    { command, transport: TransportKind.stdio },
    {
      documentSelector: [{ scheme: "file", language: "html", pattern: "**/*.shape.html" }],
      outputChannel: output,
    },
  );
  try {
    await client.start();
  } catch (err) {
    vscode.window.showErrorMessage(`Semantic Shape: the language server did not start: ${err.message || err}`);
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
  output = vscode.window.createOutputChannel("Semantic Shape");
  context.subscriptions.push(
    output,
    vscode.commands.registerCommand("semanticshape.restartServer", restart),
    vscode.workspace.onDidChangeConfiguration((event) => {
      if (event.affectsConfiguration("semanticshape.serverPath")) {
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
