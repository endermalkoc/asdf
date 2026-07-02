import * as vscode from "vscode";
import { CuspClient, TestCaseInfo, TestResultInfo, TestSuiteInfo } from "../cusp/client";

// The Cusp "Tests" activity-bar view. Two fixed roots:
//   • Tests      — the imported catalog: suites (self-nesting tree) → cases
//   • Test Runs  — the runs → per-case results
// The catalog (suites + cases) is fetched once and indexed; run results load lazily on expand.

const S = vscode.TreeItemCollapsibleState;

/** A fixed root node. */
class RootItem extends vscode.TreeItem {
  constructor(public readonly kind: "tests" | "runs") {
    super(kind === "tests" ? "Tests" : "Test Runs", S.Collapsed);
    this.contextValue = "cuspTestRoot";
    this.iconPath = new vscode.ThemeIcon(kind === "tests" ? "beaker" : "history");
  }
}

class SuiteItem extends vscode.TreeItem {
  constructor(public readonly suite: TestSuiteInfo, directCases: number) {
    super(suite.name || "(untitled suite)", S.Collapsed);
    this.contextValue = "cuspTestSuite";
    this.iconPath = new vscode.ThemeIcon("folder");
    this.tooltip = suite.description || suite.name;
    this.description = directCases > 0 ? `${directCases}` : undefined;
  }
}

class CaseItem extends vscode.TreeItem {
  constructor(public readonly tc: TestCaseInfo) {
    super(tc.title, S.None);
    this.contextValue = "cuspTestCase";
    this.iconPath = new vscode.ThemeIcon("beaker");
    this.description = [tc.type, tc.status, tc.priority !== undefined ? `P${tc.priority}` : undefined]
      .filter(Boolean)
      .join(" · ");
    this.tooltip = tc.title;
    this.command = { command: "cusp.openTestCase", title: "Open Test Case", arguments: [tc.id] };
  }
}

class RunItem extends vscode.TreeItem {
  constructor(public readonly run: { id: string; title: string; status: string; milestone?: string }) {
    super(run.title || "(untitled run)", S.Collapsed);
    this.contextValue = "cuspTestRun";
    this.description = run.status;
    this.iconPath = new vscode.ThemeIcon(iconForRunStatus(run.status));
    this.tooltip = run.milestone ? `${run.title} — ${run.milestone}` : run.title;
  }
}

class ResultItem extends vscode.TreeItem {
  constructor(result: TestResultInfo, caseTitle: string) {
    super(caseTitle, S.None);
    this.contextValue = "cuspTestResult";
    this.description = result.status;
    this.iconPath = iconForResult(result.status);
    this.tooltip = `${caseTitle} — ${result.status}`;
    // Clicking a result opens the same case-detail panel (its steps / gherkin).
    this.command = { command: "cusp.openTestCase", title: "Open Test Case", arguments: [result.caseId] };
  }
}

type TestNode = RootItem | SuiteItem | CaseItem | RunItem | ResultItem;

export class TestTreeProvider implements vscode.TreeDataProvider<TestNode> {
  private readonly _onDidChangeTreeData = new vscode.EventEmitter<void>();
  readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

  // Indexed catalog (built once per refresh from two CLI calls).
  private catalog?: Promise<{
    childSuites: Map<string, TestSuiteInfo[]>; // parentId ("" = root) → suites
    casesBySuite: Map<string, TestCaseInfo[]>;
    caseTitle: Map<string, string>;
  }>;

  constructor(private client: CuspClient) {}

  setClient(client: CuspClient): void {
    this.client = client;
  }

  refresh(): void {
    this.catalog = undefined;
    this._onDidChangeTreeData.fire();
  }

  getTreeItem(element: TestNode): vscode.TreeItem {
    return element;
  }

  private loadCatalog() {
    if (!this.catalog) {
      this.catalog = (async () => {
        const [suites, cases] = await Promise.all([this.client.testSuites(), this.client.testCases()]);
        const childSuites = new Map<string, TestSuiteInfo[]>();
        for (const s of suites) {
          const key = s.parentId ?? "";
          (childSuites.get(key) ?? childSuites.set(key, []).get(key)!).push(s);
        }
        const casesBySuite = new Map<string, TestCaseInfo[]>();
        const caseTitle = new Map<string, string>();
        for (const c of cases) {
          caseTitle.set(c.id, c.title);
          const key = c.suiteId ?? "";
          (casesBySuite.get(key) ?? casesBySuite.set(key, []).get(key)!).push(c);
        }
        return { childSuites, casesBySuite, caseTitle };
      })();
    }
    return this.catalog;
  }

  async getChildren(element?: TestNode): Promise<TestNode[]> {
    try {
      if (!element) {
        return [new RootItem("tests"), new RootItem("runs")];
      }
      if (element instanceof RootItem) {
        return element.kind === "tests" ? this.topSuites() : this.runNodes();
      }
      if (element instanceof SuiteItem) {
        const { childSuites, casesBySuite } = await this.loadCatalog();
        const kids = (childSuites.get(element.suite.id) ?? []).map(
          (s) => new SuiteItem(s, (casesBySuiteCount(casesBySuite, s.id))),
        );
        const cases = (casesBySuite.get(element.suite.id) ?? []).map((c) => new CaseItem(c));
        return [...kids, ...cases];
      }
      if (element instanceof RunItem) {
        const { caseTitle } = await this.loadCatalog();
        const results = await this.client.testResults(element.run.id);
        return results.map((r) => new ResultItem(r, caseTitle.get(r.caseId) ?? r.caseId));
      }
      return [];
    } catch (err) {
      vscode.window.showErrorMessage(`Cusp: failed to load tests — ${messageOf(err)}`);
      return [];
    }
  }

  private async topSuites(): Promise<TestNode[]> {
    const { childSuites, casesBySuite } = await this.loadCatalog();
    return (childSuites.get("") ?? []).map((s) => new SuiteItem(s, casesBySuiteCount(casesBySuite, s.id)));
  }

  private async runNodes(): Promise<TestNode[]> {
    const runs = await this.client.testRuns();
    return runs.map((r) => new RunItem(r));
  }
}

function casesBySuiteCount(m: Map<string, TestCaseInfo[]>, suiteId: string): number {
  return m.get(suiteId)?.length ?? 0;
}

function iconForRunStatus(status: string): string {
  switch (status) {
    case "complete":
      return "check-all";
    case "aborted":
      return "circle-slash";
    default:
      return "play-circle"; // active
  }
}

function iconForResult(status: string): vscode.ThemeIcon {
  switch (status) {
    case "passed":
      return new vscode.ThemeIcon("pass", new vscode.ThemeColor("testing.iconPassed"));
    case "failed":
      return new vscode.ThemeIcon("error", new vscode.ThemeColor("testing.iconFailed"));
    case "blocked":
      return new vscode.ThemeIcon("circle-slash", new vscode.ThemeColor("testing.iconErrored"));
    case "skipped":
      return new vscode.ThemeIcon("debug-step-over", new vscode.ThemeColor("testing.iconSkipped"));
    case "invalid":
      return new vscode.ThemeIcon("warning");
    default:
      return new vscode.ThemeIcon("circle-outline");
  }
}

function messageOf(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
