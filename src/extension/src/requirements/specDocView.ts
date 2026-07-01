import * as vscode from "vscode";
import * as path from "node:path";
import { CuspClient } from "../cusp/client";

interface OpenArgs {
  /** The target spec's canonical .md doc path (also the base for resolving its links). */
  docPath: string;
  anchor?: string;
  title?: string;
}

// A single reused webview panel. The extension holds no state beyond this view handle and the
// path of the doc currently shown (needed to resolve its relative cross-references); Dolt is the
// source of truth and every doc is re-rendered on demand via `cusp spec render`.
let panel: vscode.WebviewPanel | undefined;
let currentDocPath = "";
let getClientRef: (() => CuspClient) | undefined;

export function registerSpecDocView(getClient: () => CuspClient): vscode.Disposable {
  getClientRef = getClient;
  return vscode.commands.registerCommand("cusp.openSpecDoc", (arg: OpenArgs) => openDoc(arg));
}

async function openDoc(arg: OpenArgs): Promise<void> {
  if (!arg?.docPath || !getClientRef) {
    return;
  }
  let html: string;
  try {
    html = await getClientRef().renderSpecHtml(arg.docPath);
  } catch (err) {
    vscode.window.showErrorMessage(`Cusp: failed to render spec — ${messageOf(err)}`);
    return;
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
    });
    // The listener lives on the webview and survives html reloads, so it keeps handling link
    // clicks from whichever doc is currently loaded.
    panel.webview.onDidReceiveMessage((m) => onMessage(m));
  }
  panel.title = title;
  panel.webview.html = decorate(html, arg.anchor);
  panel.reveal(panel.viewColumn ?? vscode.ViewColumn.Beside, /* preserveFocus */ true);
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
  if (!isSpecPath(target.docPath)) {
    vscode.window.showInformationMessage(
      `Cusp: ${target.docPath} isn't a spec — only spec links open in this view for now.`,
    );
    return;
  }
  await openDoc({ docPath: target.docPath, anchor: target.anchor, title: path.posix.basename(target.docPath, ".md") });
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

// isSpecPath excludes the generated non-spec pages (entity/glossary/planning/index) — only spec
// docs can be re-rendered via `cusp spec render`.
function isSpecPath(p: string): boolean {
  return !(
    p.startsWith("entities/") ||
    p.startsWith("planning/") ||
    p === "glossary.md" ||
    p === "index.md" ||
    p.endsWith("/index.md")
  );
}

// decorate injects a CSP (the doc's CSS is inlined; only a nonce'd script is added) and the
// client script that scrolls to the anchor on load and routes link clicks: same-page anchors
// scroll in place, external URLs open in the browser, and relative cross-doc links post back to
// the extension to re-render the target.
function decorate(html: string, anchor?: string): string {
  const nonce = makeNonce();
  const csp =
    `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; ` +
    `style-src 'unsafe-inline'; img-src data:; script-src 'nonce-${nonce}';">`;
  const script = `<script nonce="${nonce}">${clientScript(anchor)}</script>`;
  return html.replace("<head>", `<head>\n${csp}`).replace("</body>", `${script}\n</body>`);
}

function clientScript(anchor?: string): string {
  const scrollOnLoad = anchor
    ? `var t=document.getElementById(${JSON.stringify(anchor)});if(t){t.scrollIntoView({block:'start'});}`
    : "";
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
