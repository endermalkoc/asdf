import * as vscode from "vscode";
import { CuspClient, TestCaseDetail, TestStepDetail } from "../cusp/client";

// A read-only detail panel for a test case — title, metadata chips, preconditions, and the
// ordered steps (Qase gherkin lives in each step's `action`), rendered Qase-style. Reused by
// both the Tests catalog (a case) and a run's results (the case behind a result). One shared
// webview panel, refreshed per open.

export function registerTestCaseView(getClient: () => CuspClient): vscode.Disposable {
  let panel: vscode.WebviewPanel | undefined;

  const open = async (caseId?: string) => {
    if (!caseId) {
      return;
    }
    const client = getClient();
    try {
      const [tc, steps] = await Promise.all([client.testCase(caseId), client.testCaseSteps(caseId)]);
      if (!panel) {
        panel = vscode.window.createWebviewPanel("cuspTestCase", tc.title || "Test Case", vscode.ViewColumn.Active, {
          enableScripts: false,
          retainContextWhenHidden: true,
        });
        panel.onDidDispose(() => {
          panel = undefined;
        });
      }
      panel.title = tc.title || "Test Case";
      panel.webview.html = renderCaseHtml(tc, steps);
      panel.reveal(undefined, true);
    } catch (err) {
      vscode.window.showErrorMessage(`Cusp: failed to open test case — ${messageOf(err)}`);
    }
  };

  return vscode.commands.registerCommand("cusp.openTestCase", (caseId?: string) => open(caseId));
}

function renderCaseHtml(tc: TestCaseDetail, steps: TestStepDetail[]): string {
  const chips = [
    tc.type && chip("type", tc.type),
    tc.status && chip("status", tc.status),
    tc.priority !== undefined && chip("priority", `P${tc.priority}`),
    tc.severity && chip("severity", tc.severity),
    tc.automation && chip("automation", tc.automation),
    tc.layer && chip("layer", tc.layer),
    tc.isFlaky ? chip("flaky", "yes") : "",
  ]
    .filter(Boolean)
    .join(" ");

  const section = (title: string, body: string) => (body ? `<h2>${esc(title)}</h2>${body}` : "");

  const stepsHtml =
    steps.length > 0
      ? steps
          .map((s, i) => {
            const pos = s.position ?? i + 1;
            const action = gherkin(s.action ?? "");
            const expected = s.expectedResult ? `<div class="expected"><b>Expected:</b> ${esc(s.expectedResult)}</div>` : "";
            return `<div class="step"><div class="stepno">${pos}</div><div class="stepbody">${action}${expected}</div></div>`;
          })
          .join("")
      : `<p class="muted">No steps recorded.</p>`;

  return `<!doctype html><html><head><meta charset="utf-8"><style>
    body { font-family: var(--vscode-font-family); color: var(--vscode-foreground); padding: 0 20px 30px; line-height: 1.5; }
    h1 { font-size: 1.35em; margin: 18px 0 6px; }
    h2 { font-size: 1.0em; margin: 22px 0 8px; text-transform: uppercase; letter-spacing: .04em; opacity: .8; }
    .chips { margin: 6px 0 4px; }
    .chip { display: inline-block; font-size: .8em; padding: 1px 8px; margin: 0 4px 4px 0; border-radius: 10px;
            background: var(--vscode-badge-background); color: var(--vscode-badge-foreground); }
    .chip b { font-weight: 600; opacity: .7; margin-right: 4px; }
    .muted { opacity: .6; }
    pre.pre { white-space: pre-wrap; background: var(--vscode-textCodeBlock-background); padding: 10px 12px; border-radius: 6px; }
    .step { display: flex; gap: 10px; margin: 10px 0; }
    .stepno { flex: 0 0 24px; height: 24px; text-align: center; line-height: 24px; border-radius: 50%;
              background: var(--vscode-badge-background); color: var(--vscode-badge-foreground); font-size: .85em; }
    .stepbody { flex: 1; }
    .gherkin { white-space: pre-wrap; background: var(--vscode-textCodeBlock-background); padding: 10px 12px; border-radius: 6px; }
    .gherkin .kw { color: var(--vscode-textLink-foreground); font-weight: 600; }
    .expected { margin-top: 6px; opacity: .9; }
  </style></head><body>
    <h1>${esc(tc.title)}</h1>
    <div class="chips">${chips}</div>
    ${section("Preconditions", tc.preconditions ? `<pre class="pre">${esc(tc.preconditions)}</pre>` : "")}
    ${section("Description", tc.description ? `<pre class="pre">${esc(tc.description)}</pre>` : "")}
    <h2>Steps</h2>
    ${stepsHtml}
    ${tc.path ? section("Automated test", `<pre class="pre">${esc(tc.path)}</pre>`) : ""}
  </body></html>`;
}

// gherkin renders a step's action with Given/When/Then/And/But keywords highlighted.
function gherkin(action: string): string {
  const html = esc(action).replace(
    /^(\s*)(Given|When|Then|And|But)\b/gm,
    (_m, sp: string, kw: string) => `${sp}<span class="kw">${kw}</span>`,
  );
  return `<div class="gherkin">${html}</div>`;
}

function chip(label: string, value: string): string {
  return `<span class="chip"><b>${esc(label)}</b>${esc(value)}</span>`;
}

function esc(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

function messageOf(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
