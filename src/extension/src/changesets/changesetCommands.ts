import * as vscode from "vscode";
import { CuspClient } from "../cusp/client";
import { ChangesetItem } from "./changesetTree";

// Changeset lifecycle actions — the PR verbs — wired to the existing `cusp changeset …` CLI, each
// refreshing the tree on success. New/Submit are non-destructive; Merge/Abandon confirm first
// (Merge writes to main; Abandon deletes the branch). The extension holds no state — these are thin
// calls over the same `Mutate` contract the CLI uses.

export function registerChangesetCommands(getClient: () => CuspClient, refresh: () => void): vscode.Disposable {
  const run = async (verb: string, action: () => Promise<void>) => {
    try {
      await action();
      refresh();
    } catch (err) {
      vscode.window.showErrorMessage(`Cusp: ${verb} failed — ${messageOf(err)}`);
    }
  };

  const newChangeset = async () => {
    const title = await vscode.window.showInputBox({
      prompt: "New changeset title",
      placeHolder: "e.g. tighten invoice specs",
    });
    if (!title?.trim()) {
      return;
    }
    await run("create changeset", () => getClient().startChangeset(title.trim()));
  };

  const submit = (item?: ChangesetItem) =>
    withBranch(item, (branch) => run("submit", () => getClient().submitChangeset(branch)));

  const merge = (item?: ChangesetItem) =>
    withBranch(item, async (branch) => {
      const ok = await vscode.window.showWarningMessage(
        `Merge ${branch} into main?`,
        { modal: true },
        "Merge",
      );
      if (ok === "Merge") {
        await run("merge", () => getClient().mergeChangeset(branch));
      }
    });

  const abandon = (item?: ChangesetItem) =>
    withBranch(item, async (branch) => {
      const ok = await vscode.window.showWarningMessage(
        `Abandon ${branch}? This closes the changeset and deletes its branch.`,
        { modal: true },
        "Abandon",
      );
      if (ok === "Abandon") {
        await run("abandon", () => getClient().abandonChangeset(branch));
      }
    });

  return vscode.Disposable.from(
    vscode.commands.registerCommand("cusp.newChangeset", newChangeset),
    vscode.commands.registerCommand("cusp.submitChangeset", submit),
    vscode.commands.registerCommand("cusp.mergeChangeset", merge),
    vscode.commands.registerCommand("cusp.abandonChangeset", abandon),
  );
}

// withBranch resolves the target branch from the invoking tree item (these commands are only
// contributed to changeset context menus, so an item is always present).
function withBranch(item: ChangesetItem | undefined, fn: (branch: string) => void | Promise<void>) {
  const branch = item?.changeset.branch;
  if (!branch) {
    vscode.window.showErrorMessage("Cusp: run this from a changeset in the Changesets view.");
    return;
  }
  return fn(branch);
}

function messageOf(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
