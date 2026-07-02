import * as vscode from "vscode";
import * as path from "node:path";
import { CuspClient } from "../cusp/client";

interface OpenArgs {
  /** The target spec's canonical .md doc path (also the base for resolving its links). */
  docPath: string;
  anchor?: string; // scroll to an element id (FRs)
  find?: string; // scroll to the first heading whose text contains this (stories/sections)
  title?: string;
}

// A single reused webview panel plus a back/forward history. The extension holds no doc state
// beyond this view handle, the path currently shown (to resolve its relative links), and the
// navigation history; Dolt is the source of truth and every doc is re-rendered on demand.
let panel: vscode.WebviewPanel | undefined;
let currentDocPath = "";
let getClientRef: (() => CuspClient) | undefined;
let onNavigateRef: ((docPath: string, anchor?: string) => void) | undefined;

const history: OpenArgs[] = [];
let historyIndex = -1;

export function registerSpecDocView(
  getClient: () => CuspClient,
  onNavigate: (docPath: string, anchor?: string) => void,
): vscode.Disposable {
  getClientRef = getClient;
  onNavigateRef = onNavigate;
  return vscode.Disposable.from(
    // Tree clicks navigate but don't reveal — the tree already selected the clicked node.
    vscode.commands.registerCommand("cusp.openSpecDoc", (arg: OpenArgs) => navigate(arg, false)),
    vscode.commands.registerCommand("cusp.specDocBack", () => step(-1)),
    vscode.commands.registerCommand("cusp.specDocForward", () => step(1)),
  );
}

// navigate renders and pushes onto history (dropping any forward entries). reveal follows the
// target in the tree (used for in-doc link clicks, not tree clicks).
async function navigate(arg: OpenArgs, reveal: boolean): Promise<void> {
  if (!arg?.docPath) {
    return;
  }
  if (!(await render(arg, reveal))) {
    return;
  }
  history.splice(historyIndex + 1);
  history.push(arg);
  historyIndex = history.length - 1;
  updateNavContext();
}

// step moves through history without mutating it (the back/forward buttons); it reveals.
async function step(delta: number): Promise<void> {
  const target = historyIndex + delta;
  if (target < 0 || target >= history.length) {
    return;
  }
  if (await render(history[target], true)) {
    historyIndex = target;
    updateNavContext();
  }
}

// render shows the doc in the panel; it never touches history. Returns false on failure.
async function render(arg: OpenArgs, reveal: boolean): Promise<boolean> {
  if (!getClientRef) {
    return false;
  }
  let html: string;
  try {
    html = await getClientRef().renderDocHtml(arg.docPath);
  } catch (err) {
    vscode.window.showErrorMessage(`Cusp: failed to render spec — ${messageOf(err)}`);
    return false;
  }
  currentDocPath = arg.docPath;
  const title = arg.title || path.posix.basename(arg.docPath, ".md");
  if (!panel) {
    panel = vscode.window.createWebviewPanel("cuspSpecDoc", title, vscode.ViewColumn.Beside, {
      enableScripts: true,
      retainContextWhenHidden: true,
    });
    panel.onDidDispose(() => {
      panel = undefined;
      history.length = 0;
      historyIndex = -1;
      updateNavContext();
    });
    panel.webview.onDidReceiveMessage((m) => onMessage(m));
  }
  panel.title = title;
  panel.webview.html = decorate(html, arg.anchor, arg.find);
  panel.reveal(panel.viewColumn ?? vscode.ViewColumn.Beside, /* preserveFocus */ true);
  if (reveal) {
    onNavigateRef?.(arg.docPath, arg.anchor);
  }
  return true;
}

function updateNavContext(): void {
  void vscode.commands.executeCommand("setContext", "cusp.specDocCanGoBack", historyIndex > 0);
  void vscode.commands.executeCommand("setContext", "cusp.specDocCanGoForward", historyIndex < history.length - 1);
}

async function onMessage(m: unknown): Promise<void> {
  const msg = m as { type?: string; href?: string };
  if (msg?.type !== "openLink" || typeof msg.href !== "string") {
    return;
  }
  const target = resolveHref(currentDocPath, msg.href);
  if (!target) {
    return;
  }
  if (!isRenderableDoc(target.docPath)) {
    vscode.window.showInformationMessage(
      `Cusp: ${target.docPath} has no rendered view — only spec and entity links open here.`,
    );
    return;
  }
  await navigate({ docPath: target.docPath, anchor: target.anchor, title: path.posix.basename(target.docPath, ".md") }, true);
}

// resolveHref turns a relative .html href from the rendered doc into a target .md doc path (plus
// optional anchor), resolved against the current document's directory.
function resolveHref(baseDocPath: string, href: string): { docPath: string; anchor?: string } | undefined {
  const hash = href.indexOf("#");
  const rawPath = hash >= 0 ? href.slice(0, hash) : href;
  const anchor = hash >= 0 ? href.slice(hash + 1) : undefined;
  if (!rawPath) {
    return undefined; // pure #anchor — handled in-page by the webview script
  }
  const dir = path.posix.dirname(baseDocPath);
  const resolved = path.posix.normalize(path.posix.join(dir, rawPath));
  return { docPath: resolved.replace(/\.html$/, ".md"), anchor };
}

// isRenderableDoc reports whether a doc path has a render chokepoint: spec docs and entity docs
// (entities/…) do; the generated glossary/planning/index rollup pages do not.
function isRenderableDoc(p: string): boolean {
  if (p.startsWith("entities/")) {
    return true; // entity doc → `cusp entity render`
  }
  return !(p.startsWith("planning/") || p === "glossary.md" || p === "index.md" || p.endsWith("/index.md"));
}

// decorate injects a CSP (the doc's CSS is inlined; only a nonce'd script is added) and the
// client script that scrolls to the target on load and routes link clicks: same-page anchors
// scroll in place, external URLs open in the browser, and relative cross-doc links post back to
// the extension to re-render the target.
function decorate(html: string, anchor?: string, find?: string): string {
  const nonce = makeNonce();
  const csp =
    `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; ` +
    `style-src 'unsafe-inline'; img-src data:; script-src 'nonce-${nonce}';">`;
  const script = `<script nonce="${nonce}">${clientScript(anchor, find)}</script>`;
  return html.replace("<head>", `<head>\n${csp}`).replace("</body>", `${script}\n</body>`);
}

function clientScript(anchor?: string, find?: string): string {
  let scrollOnLoad = "";
  if (anchor) {
    scrollOnLoad = `var t=document.getElementById(${JSON.stringify(anchor)});if(t){t.scrollIntoView({block:'start'});}`;
  } else if (find) {
    // Stories/sections have no stable id — scroll to the first heading containing the text.
    scrollOnLoad =
      `var hs=document.querySelectorAll('h1,h2,h3,h4,h5,h6');` +
      `for(var i=0;i<hs.length;i++){if((hs[i].textContent||'').indexOf(${JSON.stringify(find)})>=0){hs[i].scrollIntoView({block:'start'});break;}}`;
  }
  return `
const vscode = acquireVsCodeApi();
window.addEventListener('load', function () { ${scrollOnLoad} });
document.addEventListener('click', function (ev) {
  var a = ev.target && ev.target.closest ? ev.target.closest('a') : null;
  if (!a) { return; }
  var href = a.getAttribute('href');
  if (!href) { return; }
  if (href.charAt(0) === '#') {
    ev.preventDefault();
    var el = document.getElementById(href.slice(1));
    if (el) { el.scrollIntoView({ block: 'start' }); }
    return;
  }
  if (/^[a-z][a-z0-9+.-]*:/i.test(href)) { return; } // http:, https:, mailto: — host opens externally
  ev.preventDefault();
  vscode.postMessage({ type: 'openLink', href: href });
});
`;
}

function makeNonce(): string {
  const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";
  let s = "";
  for (let i = 0; i < 32; i++) {
    s += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return s;
}

function messageOf(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
