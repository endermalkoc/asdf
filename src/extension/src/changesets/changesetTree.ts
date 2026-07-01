import * as vscode from "vscode";
import { Changeset, CuspClient } from "../cusp/client";

/** A single changeset row in the Cusp activity-bar tree. */
export class ChangesetItem extends vscode.TreeItem {
  constructor(public readonly changeset: Changeset) {
    super(changeset.branch, vscode.TreeItemCollapsibleState.None);
    this.description = changeset.status;
    this.tooltip = changeset.title || changeset.branch;
    this.contextValue = "cuspChangeset";
    this.iconPath = new vscode.ThemeIcon(iconForStatus(changeset.status));
  }
}

function iconForStatus(status: string): string {
  switch (status) {
    case "merged":
      return "git-merge";
    case "approved":
      return "check";
    case "denied":
    case "closed":
      return "circle-slash";
    case "changes_requested":
      return "request-changes";
    default:
      return "git-pull-request"; // draft / open
  }
}

export class ChangesetTreeProvider implements vscode.TreeDataProvider<ChangesetItem> {
  private readonly _onDidChangeTreeData = new vscode.EventEmitter<void>();
  readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

  constructor(private client: CuspClient) {}

  /** Swap the transport (e.g. after a settings change) and let callers refresh. */
  setClient(client: CuspClient): void {
    this.client = client;
  }

  refresh(): void {
    this._onDidChangeTreeData.fire();
  }

  getTreeItem(element: ChangesetItem): vscode.TreeItem {
    return element;
  }

  async getChildren(element?: ChangesetItem): Promise<ChangesetItem[]> {
    if (element) {
      return []; // flat list for now; entity-level children come with the diff slice
    }
    try {
      const changesets = await this.client.listChangesets();
      return changesets.map((c) => new ChangesetItem(c));
    } catch (err) {
      vscode.window.showErrorMessage(`Cusp: failed to list changesets — ${messageOf(err)}`);
      return [];
    }
  }
}

function messageOf(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
