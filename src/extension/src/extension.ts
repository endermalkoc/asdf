import * as vscode from "vscode";
import { CliCuspClient } from "./cusp/cliClient";
import { ChangesetTreeProvider } from "./changesets/changesetTree";

export function activate(context: vscode.ExtensionContext): void {
  const client = new CliCuspClient(resolveCliPath(), resolveWorkspaceDir());
  const changesets = new ChangesetTreeProvider(client);

  context.subscriptions.push(
    vscode.window.registerTreeDataProvider("cuspChangesets", changesets),
    vscode.commands.registerCommand("cusp.refreshChangesets", () => changesets.refresh()),
  );
}

export function deactivate(): void {
  // Nothing to tear down — the extension holds no state; Dolt is the source of truth.
}

function resolveCliPath(): string {
  return vscode.workspace.getConfiguration("cusp").get<string>("cliPath")?.trim() || "cusp";
}

function resolveWorkspaceDir(): string {
  const configured = vscode.workspace.getConfiguration("cusp").get<string>("workspaceFolder")?.trim();
  if (configured) {
    return configured;
  }
  const folders = vscode.workspace.workspaceFolders;
  if (folders && folders.length > 0) {
    return folders[0].uri.fsPath;
  }
  return process.cwd();
}
