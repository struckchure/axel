import { execFile } from "node:child_process";
import { promisify } from "node:util";
import * as vscode from "vscode";
import {
  LanguageClient,
  type LanguageClientOptions,
  type ServerOptions,
  TransportKind,
} from "vscode-languageclient/node";

const execFileAsync = promisify(execFile);

let client: LanguageClient | undefined;

/**
 * Resolve the `axel` binary: the `axel.path` setting if set, otherwise `axel`
 * on the PATH. Returns undefined if the resolved binary can't be executed
 * (e.g. not installed).
 */
async function resolveAxel(): Promise<string | undefined> {
  const configured = vscode.workspace.getConfiguration("axel").get<string>("path")?.trim();
  const bin = configured && configured.length > 0 ? configured : "axel";
  try {
    // `axel version` is a subcommand (there is no --version flag); a clean exit
    // confirms the binary is present and runnable.
    await execFileAsync(bin, ["version"]);
    return bin;
  } catch {
    return undefined;
  }
}

async function startClient(context: vscode.ExtensionContext): Promise<void> {
  const bin = await resolveAxel();
  if (!bin) {
    void vscode.window.showErrorMessage(
      "Axel language server not found. Install the axel CLI and make sure `axel` is on your " +
        "PATH, or set `axel.path` in your settings. Syntax highlighting still works.",
    );
    return;
  }

  const serverOptions: ServerOptions = {
    run: { command: bin, args: ["lsp"], transport: TransportKind.stdio },
    debug: { command: bin, args: ["lsp"], transport: TransportKind.stdio },
  };
  const clientOptions: LanguageClientOptions = {
    documentSelector: [
      { scheme: "file", language: "asl" },
      { scheme: "file", language: "aql" },
    ],
  };

  client = new LanguageClient("axel", "Axel Language Server", serverOptions, clientOptions);
  await client.start();
  context.subscriptions.push(client);
}

async function stopClient(): Promise<void> {
  if (client) {
    await client.stop();
    client = undefined;
  }
}

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  // Register the command first so it always exists — even if the server fails to
  // start, the restart command is how the user retries after fixing it.
  context.subscriptions.push(
    vscode.commands.registerCommand("axel.restartServer", async () => {
      await stopClient();
      await startClient(context);
    }),
  );
  try {
    await startClient(context);
  } catch (err) {
    void vscode.window.showErrorMessage(`Axel: failed to start language server: ${String(err)}`);
  }
}

export function deactivate(): Thenable<void> | undefined {
  return client?.stop();
}
