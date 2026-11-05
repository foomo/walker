// Semantic Shape for VS Code: starts semanticshape-lsp (go/cmd/semanticshape-lsp
// of the walker repository, bundled in server/) for semantic shape files,
// *.semanticshape.html. They stay
// html documents, so VS Code's own html support keeps working next to it; the
// server only acts on shape files inside a shape root - a directory with
// groups/ or default.semanticshape.html.
const fs = require("fs");
const os = require("os");
const path = require("path");
const vscode = require("vscode");
const { LanguageClient, TransportKind } = require("vscode-languageclient/node");

const installHint = "the extension has no server for this platform - build it with `mise run editors:vscode`, or install one with: go -C go install ./cmd/semanticshape-lsp";

/** @type {LanguageClient | undefined} */
let client;
/** @type {vscode.OutputChannel | undefined} */
let output;
/** @type {string} the directory of the extension, with the bundled servers */
let extensionPath = "";

function isExecutable(file) {
  try {
    fs.accessSync(file, fs.constants.X_OK);
    return fs.statSync(file).isFile();
  } catch {
    return false;
  }
}

// bundledServer is the binary packaged with the extension for this platform -
// server/<platform>-<arch>, built by `mise run editors:vscode`. A vsix doesn't
// keep the executable bit reliably, so it is set here.
function bundledServer(extensionPath) {
  const exe = process.platform === "win32" ? "semanticshape-lsp.exe" : "semanticshape-lsp";
  const file = path.join(extensionPath, "server", `${process.platform}-${process.arch}`, exe);
  if (!fs.existsSync(file)) {
    return undefined;
  }
  if (process.platform !== "win32" && !isExecutable(file)) {
    try {
      fs.chmodSync(file, 0o755);
    } catch {
      return undefined;
    }
  }
  return file;
}

// resolveServer finds the binary: a serverPath setting as it is, else the
// bundled one, else semanticshape-lsp on the PATH and in go's bin directories -
// VS Code started from the Dock often has a PATH without them
function resolveServer(setting, extensionPath) {
  if (!setting) {
    const bundled = bundledServer(extensionPath);
    if (bundled) {
      return bundled;
    }
  }
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
  const command = resolveServer(setting, extensionPath);
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
      documentSelector: [{ scheme: "file", language: "html", pattern: "**/*.semanticshape.html" }],
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
  extensionPath = context.extensionPath;
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
