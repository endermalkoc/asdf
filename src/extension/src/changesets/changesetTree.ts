import * as vscode from "vscode";
import { Changeset, CuspClient, EntityDiff } from "../cusp/client";

/** An abandoned changeset has no branch to diff, so it isn't reviewable (merged still is). */
export function isReviewable(status: string): boolean {
  return status !== "closed";
}

// The tree-item contextValue drives which menu actions show: active changesets get the full set
// (review, verdict, submit/merge/abandon); merged ones are viewable-only (their diff, no
// lifecycle); abandoned ones are inert.
function changesetContext(status: string): string {
  if (status === "closed") {
    return "cuspChangesetClosed";
  }
  if (status === "merged") {
    return "cuspChangesetMerged";
  }
  return "cuspChangeset"; // active: draft | open | changes_requested | approved
}

/** A single changeset row in the Cusp activity-bar tree. Expands to its changed entities. */
export class ChangesetItem extends vscode.TreeItem {
  constructor(public readonly changeset: Changeset) {
    const reviewable = isReviewable(changeset.status);
    super(
      changeset.branch,
      reviewable ? vscode.TreeItemCollapsibleState.Collapsed : vscode.TreeItemCollapsibleState.None,
    );
    this.description = changeset.status;
    this.tooltip = changeset.title || changeset.branch;
    // The contextValue gates the menu actions per status; an abandoned changeset gets no click
    // command either, so clicking it is a silent no-op (no diff error, no popup).
    this.contextValue = changesetContext(changeset.status);
    this.iconPath = new vscode.ThemeIcon(iconForStatus(changeset.status));
    if (reviewable) {
      // Clicking the label opens the review surface; the twistie still expands the entity list.
      this.command = { command: "cusp.reviewChangeset", title: "Review", arguments: [changeset.branch] };
    }
  }
}

/** One changed entity under a changeset — a leaf that opens the changeset's review surface. */
export class EntityDiffItem extends vscode.TreeItem {
  constructor(public readonly branch: string, public readonly diff: EntityDiff) {
    super(diff.subjectRef, vscode.TreeItemCollapsibleState.None);
    this.description = diff.changeType;
    this.contextValue = "cuspEntityDiff";
    this.iconPath = new vscode.ThemeIcon(iconForChange(diff.changeType));
    this.tooltip = (diff.fields ?? []).map((f) => `${f.name}: ${f.base} → ${f.head}`).join("\n") || diff.changeType;
    this.command = { command: "cusp.reviewChangeset", title: "Review", arguments: [branch] };
  }
}

type ChangesetNode = ChangesetItem | EntityDiffItem;

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

function iconForChange(changeType: string): string {
  switch (changeType) {
    case "added":
      return "diff-added";
    case "removed":
      return "diff-removed";
    default:
      return "diff-modified";
  }
}

export class ChangesetTreeProvider implements vscode.TreeDataProvider<ChangesetNode> {
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

  getTreeItem(element: ChangesetNode): vscode.TreeItem {
    return element;
  }

  async getChildren(element?: ChangesetNode): Promise<ChangesetNode[]> {
    if (element instanceof EntityDiffItem) {
      return [];
    }
    if (element instanceof ChangesetItem) {
      if (!isReviewable(element.changeset.status)) {
        return []; // abandoned — no branch to diff
      }
      try {
        const diff = await this.client.diff(element.changeset.branch);
        return diff.entities.map((d) => new EntityDiffItem(element.changeset.branch, d));
      } catch {
        return []; // an empty/errored changeset has no diff — leave it childless
      }
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
