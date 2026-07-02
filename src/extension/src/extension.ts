import * as vscode from "vscode";
import { CliCuspClient } from "./cusp/cliClient";
import { ChangesetTreeProvider } from "./changesets/changesetTree";
import { RequirementsTreeProvider } from "./requirements/requirementsTree";
import { EntitiesTreeProvider } from "./entities/entitiesTree";
import { registerSpecDocView } from "./requirements/specDocView";
import { registerReviewView } from "./changesets/reviewView";

interface Filterable {
  readonly filterText: string;
  setFilter(q: string): void;
}

export function activate(context: vscode.ExtensionContext): void {
  const makeClient = () => new CliCuspClient(resolveCliPath(), resolveWorkspaceDir());
  let client = makeClient();

  const changesets = new ChangesetTreeProvider(client);
  const requirements = new RequirementsTreeProvider(client);
  const entities = new EntitiesTreeProvider(client);

  // TreeViews (not bare providers) so we can reveal() the node the doc panel navigates to, and get
  // a built-in Collapse All button.
  const reqView = vscode.window.createTreeView("cuspRequirements", {
    treeDataProvider: requirements,
    showCollapseAll: true,
  });
  const entitiesView = vscode.window.createTreeView("cuspEntities", {
    treeDataProvider: entities,
    showCollapseAll: true,
  });

  // Reflect a view's active filter: a "Filter: …" subtitle + a context key that shows its Clear button.
  const applyReqFilterUI = (q: string) => {
    reqView.description = q ? `Filter: ${q}` : undefined;
    void vscode.commands.executeCommand("setContext", "cusp.requirementsFiltered", q.length > 0);
  };
  const applyEntFilterUI = (q: string) => {
    entitiesView.description = q ? `Filter: ${q}` : undefined;
    void vscode.commands.executeCommand("setContext", "cusp.entitiesFiltered", q.length > 0);
  };

  // A search command: prompt for text, filter the provider, reflect it in the view.
  const searchCommand = (provider: Filterable, apply: (q: string) => void, what: string) => async () => {
    const q = await vscode.window.showInputBox({ prompt: `Filter ${what}`, value: provider.filterText });
    if (q === undefined) {
      return; // cancelled — leave the current filter unchanged
    }
    provider.setFilter(q);
    apply(q.trim());
  };

  // Doc-panel navigation reveals the target in whichever tree owns it.
  const revealInTree = async (docPath: string, anchor?: string) => {
    try {
      if (docPath.startsWith("entities/")) {
        const node = await entities.find(docPath);
        if (node) {
          await entitiesView.reveal(node, { select: true, focus: false, expand: true });
        }
      } else {
        const node = await requirements.find(docPath, anchor);
        if (node) {
          await reqView.reveal(node, { select: true, focus: false, expand: true });
        }
      }
    } catch {
      /* the view may be hidden — reveal is best-effort */
    }
  };

  context.subscriptions.push(
    vscode.window.registerTreeDataProvider("cuspChangesets", changesets),
    reqView,
    entitiesView,
    vscode.commands.registerCommand("cusp.refreshChangesets", () => changesets.refresh()),
    vscode.commands.registerCommand("cusp.refreshRequirements", () => requirements.refresh()),
    vscode.commands.registerCommand("cusp.refreshEntities", () => entities.refresh()),
    vscode.commands.registerCommand(
      "cusp.searchRequirements",
      searchCommand(requirements, applyReqFilterUI, "requirements — matches FR keys, statements, and spec/story/section titles"),
    ),
    vscode.commands.registerCommand("cusp.clearRequirementsSearch", () => {
      requirements.setFilter("");
      applyReqFilterUI("");
    }),
    vscode.commands.registerCommand(
      "cusp.searchEntities",
      searchCommand(entities, applyEntFilterUI, "entities — matches names, relationships, and section titles"),
    ),
    vscode.commands.registerCommand("cusp.clearEntitiesSearch", () => {
      entities.setFilter("");
      applyEntFilterUI("");
    }),
    registerSpecDocView(() => client, revealInTree),
    registerReviewView(() => client),
    // Rebuild the transport when the relevant settings change — no reload needed.
    vscode.workspace.onDidChangeConfiguration((e) => {
      if (e.affectsConfiguration("cusp.cliPath") || e.affectsConfiguration("cusp.workspaceFolder")) {
        client = makeClient();
        changesets.setClient(client);
        requirements.setClient(client);
        entities.setClient(client);
        changesets.refresh();
        requirements.refresh();
        entities.refresh();
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
