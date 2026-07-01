import * as vscode from "vscode";
import { CliCuspClient } from "./cusp/cliClient";
import { ChangesetTreeProvider } from "./changesets/changesetTree";
import { RequirementsTreeProvider } from "./requirements/requirementsTree";
import { registerSpecDocView } from "./requirements/specDocView";

export function activate(context: vscode.ExtensionContext): void {
  const makeClient = () => new CliCuspClient(resolveCliPath(), resolveWorkspaceDir());
  let client = makeClient();

  const changesets = new ChangesetTreeProvider(client);
  const requirements = new RequirementsTreeProvider(client);

  context.subscriptions.push(
    vscode.window.registerTreeDataProvider("cuspChangesets", changesets),
    vscode.window.registerTreeDataProvider("cuspRequirements", requirements),
    vscode.commands.registerCommand("cusp.refreshChangesets", () => changesets.refresh()),
    vscode.commands.registerCommand("cusp.refreshRequirements", () => requirements.refresh()),
    registerSpecDocView(() => client),
    // Rebuild the transport when the relevant settings change — no reload needed.
    vscode.workspace.onDidChangeConfiguration((e) => {
      if (e.affectsConfiguration("cusp.cliPath") || e.affectsConfiguration("cusp.workspaceFolder")) {
        client = makeClient();
        changesets.setClient(client);
        requirements.setClient(client);
        changesets.refresh();
        requirements.refresh();
      }
    }),
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
