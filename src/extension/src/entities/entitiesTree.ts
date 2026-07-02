import * as vscode from "vscode";
import { CuspClient, EntityTreeNode, EntityRel } from "../cusp/client";

type Kind = "entity" | "category" | "relationship" | "section";

// A node in a STABLE tree built once per load (stable id + object identity → TreeView.reveal works).
interface Node {
  kind: Kind;
  id: string;
  label: string;
  description?: string;
  tooltip?: string;
  icon?: string;
  docPath?: string; // the doc to open (relationship → the OTHER entity's doc)
  find?: string; // section: heading text to scroll to
  openTitle?: string;
  parent?: Node;
  children: Node[];
}

export class EntitiesTreeProvider implements vscode.TreeDataProvider<Node> {
  private readonly _onDidChangeTreeData = new vscode.EventEmitter<void>();
  readonly onDidChangeTreeData = this._onDidChangeTreeData.event;
  private roots: Node[] | undefined;
  private loading: Promise<Node[]> | undefined;
  private filter = "";
  private visible: Set<string> | undefined;

  constructor(private client: CuspClient) {}

  setClient(client: CuspClient): void {
    this.client = client;
  }

  refresh(): void {
    this.roots = undefined;
    this.loading = undefined;
    this.visible = undefined;
    this._onDidChangeTreeData.fire();
  }

  get filterText(): string {
    return this.filter;
  }

  setFilter(q: string): void {
    this.filter = q.trim().toLowerCase();
    this.visible = undefined;
    this._onDidChangeTreeData.fire();
  }

  getTreeItem(node: Node): vscode.TreeItem {
    const leaf = node.kind === "relationship" || node.kind === "section";
    const state = leaf
      ? vscode.TreeItemCollapsibleState.None
      : this.visible
        ? vscode.TreeItemCollapsibleState.Expanded
        : vscode.TreeItemCollapsibleState.Collapsed;
    const item = new vscode.TreeItem(node.label, state);
    item.id = node.id;
    item.description = node.description;
    if (node.tooltip) {
      item.tooltip = node.tooltip;
    }
    item.contextValue = `cusp-ent-${node.kind}`;
    item.iconPath = new vscode.ThemeIcon(node.icon ?? iconFor(node.kind));
    if (node.docPath) {
      item.command = {
        command: "cusp.openSpecDoc",
        title: "Open",
        arguments: [{ docPath: node.docPath, find: node.find, title: node.openTitle }],
      };
    }
    return item;
  }

  getChildren(node?: Node): Node[] | Promise<Node[]> {
    if (node) {
      return this.applyFilter(node.children);
    }
    return this.ensureRoots().then((roots) => {
      if (this.filter && !this.visible) {
        this.visible = this.computeVisible(roots);
      }
      return this.applyFilter(roots);
    });
  }

  getParent(node: Node): Node | undefined {
    return node.parent;
  }

  // find locates an entity by its doc path — the element handed to TreeView.reveal when the doc
  // panel navigates to an entity (e.g. following a relationship or a cross-reference link).
  async find(docPath: string): Promise<Node | undefined> {
    const roots = await this.ensureRoots();
    return roots.find((e) => e.docPath === docPath);
  }

  private applyFilter(nodes: Node[]): Node[] {
    return this.visible ? nodes.filter((n) => this.visible!.has(n.id)) : nodes;
  }

  private computeVisible(roots: Node[]): Set<string> {
    const vis = new Set<string>();
    const walk = (node: Node): boolean => {
      let keep = `${node.label} ${node.description ?? ""}`.toLowerCase().includes(this.filter);
      for (const child of node.children) {
        if (walk(child)) {
          keep = true;
        }
      }
      if (keep) {
        vis.add(node.id);
      }
      return keep;
    };
    for (const r of roots) {
      walk(r);
    }
    return vis;
  }

  private ensureRoots(): Promise<Node[]> {
    if (this.roots) {
      return Promise.resolve(this.roots);
    }
    if (!this.loading) {
      this.loading = this.client
        .entitiesTree()
        .then((ents) => (this.roots = (ents ?? []).map(buildEntity)))
        .catch((err) => {
          vscode.window.showErrorMessage(`Cusp: failed to load entities — ${messageOf(err)}`);
          this.loading = undefined;
          return [];
        });
    }
    return this.loading;
  }
}

function buildEntity(e: EntityTreeNode): Node {
  const entity: Node = {
    kind: "entity",
    id: `entity:${e.docPath}`,
    label: e.name,
    tooltip: e.docPath,
    docPath: e.docPath,
    openTitle: e.name,
    children: [],
  };
  const cats: Node[] = [];

  const rels = e.relationships ?? [];
  if (rels.length) {
    const cat = categoryNode("Relationships", `entcat:${e.docPath}:rels`, "references", entity);
    cat.children = rels.map((r, i) => relNode(r, i, e.docPath, cat));
    cats.push(cat);
  }

  const sections = e.sections ?? [];
  if (sections.length) {
    const cat = categoryNode("Sections", `entcat:${e.docPath}:sections`, "note", entity);
    cat.children = sections.map((s) => ({
      kind: "section" as Kind,
      id: `entsection:${e.docPath}:${s.key}`,
      label: s.title,
      docPath: e.docPath,
      find: s.title, // scroll to the section heading
      openTitle: `${s.title} — ${e.name}`,
      parent: cat,
      children: [],
    }));
    cats.push(cat);
  }

  entity.children = cats;
  return entity;
}

function relNode(r: EntityRel, i: number, entityDocPath: string, parent: Node): Node {
  const arrow = r.outgoing ? "→" : "←";
  return {
    kind: "relationship",
    id: `entrel:${entityDocPath}:${i}`,
    label: r.name,
    description: `${arrow} ${r.cardinality ?? ""}`.trim(),
    tooltip: `${arrow} ${r.name}${r.cardinality ? ` (${r.cardinality})` : ""}`,
    icon: r.outgoing ? "arrow-right" : "arrow-left",
    docPath: r.docPath, // clicking navigates to the related entity's doc
    openTitle: r.name,
    parent,
    children: [],
  };
}

function categoryNode(label: string, id: string, icon: string, parent: Node): Node {
  return { kind: "category", id, label, icon, parent, children: [] };
}

function iconFor(kind: Kind): string {
  switch (kind) {
    case "entity":
      return "symbol-class";
    case "category":
      return "list-tree";
    case "relationship":
      return "references";
    default:
      return "note"; // section
  }
}

function messageOf(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
