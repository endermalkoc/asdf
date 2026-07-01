import * as vscode from "vscode";
import { CuspClient, DomainNode, SpecNode, GroupNode, ReqNode } from "../cusp/client";

// The tree is a discriminated union of the four levels: domain → spec → group → requirement.
type Node =
  | { kind: "domain"; domain: DomainNode }
  | { kind: "spec"; spec: SpecNode }
  | { kind: "group"; spec: SpecNode; group: GroupNode }
  | { kind: "req"; spec: SpecNode; req: ReqNode };

export class RequirementsTreeProvider implements vscode.TreeDataProvider<Node> {
  private readonly _onDidChangeTreeData = new vscode.EventEmitter<void>();
  readonly onDidChangeTreeData = this._onDidChangeTreeData.event;
  private tree: DomainNode[] | undefined;

  constructor(private client: CuspClient) {}

  setClient(client: CuspClient): void {
    this.client = client;
  }

  refresh(): void {
    this.tree = undefined;
    this._onDidChangeTreeData.fire();
  }

  getTreeItem(node: Node): vscode.TreeItem {
    const collapsed = vscode.TreeItemCollapsibleState.Collapsed;
    if (node.kind === "domain") {
      const item = new vscode.TreeItem(node.domain.name, collapsed);
      item.iconPath = new vscode.ThemeIcon("folder");
      item.contextValue = "cuspDomain";
      return item;
    }
    if (node.kind === "spec") {
      const s = node.spec;
      const item = new vscode.TreeItem(s.prefix || s.title, collapsed);
      item.description = s.prefix ? s.title : undefined;
      item.tooltip = s.docPath;
      item.iconPath = new vscode.ThemeIcon("file");
      item.contextValue = "cuspSpec";
      // Clicking a spec opens its rendered doc at the top.
      item.command = openCommand(s, undefined, s.title);
      return item;
    }
    if (node.kind === "group") {
      const item = new vscode.TreeItem(node.group.title, collapsed);
      item.iconPath = new vscode.ThemeIcon("symbol-namespace");
      item.contextValue = "cuspGroup";
      return item;
    }
    // requirement leaf
    const r = node.req;
    const item = new vscode.TreeItem(r.frKey, vscode.TreeItemCollapsibleState.None);
    item.description = r.statement;
    item.tooltip = new vscode.MarkdownString(`**${r.frKey}**\n\n${r.statement}`);
    item.iconPath = new vscode.ThemeIcon("symbol-field");
    item.contextValue = "cuspRequirement";
    // Clicking an FR opens its parent spec's doc, scrolled to the FR.
    item.command = openCommand(node.spec, r.frKey.toLowerCase(), `${r.frKey} — ${node.spec.title}`);
    return item;
  }

  async getChildren(node?: Node): Promise<Node[]> {
    try {
      if (!node) {
        if (!this.tree) {
          this.tree = await this.client.requirementsTree();
        }
        return this.tree.map((domain) => ({ kind: "domain", domain }));
      }
      if (node.kind === "domain") {
        return (node.domain.specs ?? []).map((spec) => ({ kind: "spec", spec }));
      }
      if (node.kind === "spec") {
        const groups: Node[] = (node.spec.groups ?? []).map((group) => ({ kind: "group", spec: node.spec, group }));
        const ungrouped: Node[] = (node.spec.requirements ?? []).map((req) => ({ kind: "req", spec: node.spec, req }));
        return [...groups, ...ungrouped];
      }
      if (node.kind === "group") {
        return (node.group.requirements ?? []).map((req) => ({ kind: "req", spec: node.spec, req }));
      }
      return []; // requirement leaf
    } catch (err) {
      vscode.window.showErrorMessage(`Cusp: failed to load requirements — ${messageOf(err)}`);
      return [];
    }
  }
}

// openCommand builds the tree-item command that opens a spec's rendered doc (optionally scrolled
// to an FR anchor). The spec is addressed by its doc path — which `cusp spec render` resolves and
// which also serves as the base for resolving the doc's own relative cross-references.
function openCommand(spec: SpecNode, anchor: string | undefined, title: string): vscode.Command {
  return {
    command: "cusp.openSpecDoc",
    title: "Open",
    arguments: [{ docPath: spec.docPath, anchor, title }],
  };
}

function messageOf(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
