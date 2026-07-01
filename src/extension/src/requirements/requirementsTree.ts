import * as vscode from "vscode";
import { CuspClient, DomainNode, ReqNode } from "../cusp/client";

type Kind = "domain" | "spec" | "group" | "req";

// A node in a STABLE tree built once per load. Stable object identity + a stable `id` are what
// let TreeView.reveal() walk from an element up through getParent() and expand/select it.
interface Node {
  kind: Kind;
  id: string;
  label: string;
  description?: string;
  tooltip?: string;
  docPath?: string; // spec + req (a req carries its spec's doc path)
  anchor?: string; // req: frKey.toLowerCase()
  openTitle?: string; // webview panel title when opened
  parent?: Node;
  children: Node[];
}

export class RequirementsTreeProvider implements vscode.TreeDataProvider<Node> {
  private readonly _onDidChangeTreeData = new vscode.EventEmitter<void>();
  readonly onDidChangeTreeData = this._onDidChangeTreeData.event;
  private roots: Node[] | undefined;
  private loading: Promise<Node[]> | undefined;

  constructor(private client: CuspClient) {}

  setClient(client: CuspClient): void {
    this.client = client;
  }

  refresh(): void {
    this.roots = undefined;
    this.loading = undefined;
    this._onDidChangeTreeData.fire();
  }

  getTreeItem(node: Node): vscode.TreeItem {
    const item = new vscode.TreeItem(
      node.label,
      node.kind === "req" ? vscode.TreeItemCollapsibleState.None : vscode.TreeItemCollapsibleState.Collapsed,
    );
    item.id = node.id;
    item.description = node.description;
    if (node.tooltip) {
      item.tooltip = node.kind === "req" ? new vscode.MarkdownString(node.tooltip) : node.tooltip;
    }
    item.contextValue = `cusp-${node.kind}`;
    item.iconPath = new vscode.ThemeIcon(iconFor(node.kind));
    if (node.docPath) {
      // Specs open at the top; FRs open scrolled to their anchor.
      item.command = {
        command: "cusp.openSpecDoc",
        title: "Open",
        arguments: [{ docPath: node.docPath, anchor: node.anchor, title: node.openTitle }],
      };
    }
    return item;
  }

  getChildren(node?: Node): Node[] | Promise<Node[]> {
    return node ? node.children : this.ensureRoots();
  }

  getParent(node: Node): Node | undefined {
    return node.parent;
  }

  // find locates a spec (by doc path) or, when an anchor is given, the FR under it — the element
  // handed to TreeView.reveal so a link/back-forward navigation follows in the tree.
  async find(docPath: string, anchor?: string): Promise<Node | undefined> {
    const roots = await this.ensureRoots();
    for (const domain of roots) {
      for (const spec of domain.children) {
        if (spec.docPath !== docPath) {
          continue;
        }
        return anchor ? findReq(spec, anchor) ?? spec : spec;
      }
    }
    return undefined;
  }

  private ensureRoots(): Promise<Node[]> {
    if (this.roots) {
      return Promise.resolve(this.roots);
    }
    if (!this.loading) {
      this.loading = this.client
        .requirementsTree()
        .then((domains) => (this.roots = buildNodes(domains)))
        .catch((err) => {
          vscode.window.showErrorMessage(`Cusp: failed to load requirements — ${messageOf(err)}`);
          this.loading = undefined;
          return [];
        });
    }
    return this.loading;
  }
}

function buildNodes(domains: DomainNode[]): Node[] {
  return (domains ?? []).map((d) => {
    const domain: Node = { kind: "domain", id: `domain:${d.slug}`, label: d.name, children: [] };
    domain.children = (d.specs ?? []).map((s) => {
      const specTitle = s.title || s.prefix || s.docPath;
      const spec: Node = {
        kind: "spec",
        id: `spec:${s.docPath}`,
        label: s.prefix || s.title,
        description: s.prefix ? s.title : undefined,
        tooltip: s.docPath,
        docPath: s.docPath,
        openTitle: specTitle,
        parent: domain,
        children: [],
      };
      const mkReq = (r: ReqNode, parent: Node): Node => ({
        kind: "req",
        id: `req:${r.frKey}`,
        label: r.frKey,
        description: r.statement,
        tooltip: `**${r.frKey}**\n\n${r.statement}`,
        docPath: s.docPath,
        anchor: r.frKey.toLowerCase(),
        openTitle: `${r.frKey} — ${specTitle}`,
        parent,
        children: [],
      });
      const groups: Node[] = (s.groups ?? []).map((g) => {
        const group: Node = { kind: "group", id: `group:${s.docPath}#${g.title}`, label: g.title, parent: spec, children: [] };
        group.children = (g.requirements ?? []).map((r) => mkReq(r, group));
        return group;
      });
      const ungrouped = (s.requirements ?? []).map((r) => mkReq(r, spec));
      spec.children = [...groups, ...ungrouped];
      return spec;
    });
    return domain;
  });
}

function findReq(spec: Node, anchor: string): Node | undefined {
  for (const child of spec.children) {
    if (child.kind === "req" && child.anchor === anchor) {
      return child;
    }
    if (child.kind === "group") {
      const hit = child.children.find((r) => r.anchor === anchor);
      if (hit) {
        return hit;
      }
    }
  }
  return undefined;
}

function iconFor(kind: Kind): string {
  switch (kind) {
    case "domain":
      return "folder";
    case "spec":
      return "file";
    case "group":
      return "symbol-namespace";
    default:
      return "symbol-field";
  }
}

function messageOf(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
