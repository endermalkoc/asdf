import * as vscode from "vscode";
import { AddCommentInput, Comment, CuspClient, Verdict } from "../cusp/client";
import { isReviewable } from "./changesetTree";

// The changeset review surface. The diff itself is VS Code's **native diff editor**
// (`vscode.diff`) over two virtual documents — the affected spec rendered as Markdown at the
// base (`main`) and at the head (the changeset branch) — so we reuse VS Code's gutters,
// side-by-side/inline toggle, navigation and coloring rather than hand-rolling a diff. Review
// comments (`cusp comment ls`) hang off the head side as native CommentThreads, anchored to a
// requirement via the rendered `^fr-key` block anchors (so a comment binds to a requirement,
// not a fragile line number). Replies/resolve write back through `cusp comment …`; a verdict
// through `cusp review`. The extension holds no state — Dolt stays the source of truth.

const SCHEME = "cusp-diff";

interface HeadDoc {
  branch: string;
  docRef: string;
  keyLine: Map<string, number>; // FR-KEY (upper) → line of its `^anchor` in the head markdown
  docSubject?: string; // for a whole-doc subject (an entity doc has no per-line anchors)
}

interface Anchor {
  branch: string;
  subject?: string; // TYPE:key passed to `cusp comment add --subject`
  rootCommentId?: string; // once the thread has a root comment, replies target it
}

// A command may be invoked with an explicit branch string (the tree item's `.command`), a tree
// item (a view/item menu action passes the item), or nothing (view-title / palette → pick one).
type ChangesetArg = string | { changeset?: { branch?: string } } | undefined;

function argBranch(arg: ChangesetArg): string | undefined {
  if (typeof arg === "string") {
    return arg;
  }
  return arg?.changeset?.branch;
}

export function registerReviewView(getClient: () => CuspClient): vscode.Disposable {
  const content = new Map<string, string>(); // uri.toString() → rendered markdown
  const heads = new Map<string, HeadDoc>(); // head uri.toString() → its anchor map
  const anchors = new Map<vscode.CommentThread, Anchor>();

  const onDidChange = new vscode.EventEmitter<vscode.Uri>();
  const contentProvider: vscode.TextDocumentContentProvider = {
    onDidChange: onDidChange.event,
    provideTextDocumentContent: (uri) => content.get(uri.toString()) ?? "",
  };

  const controller = vscode.comments.createCommentController("cusp-review", "Cusp Review");
  controller.commentingRangeProvider = {
    provideCommentingRanges: (document) =>
      isHeadUri(document.uri) ? [new vscode.Range(0, 0, Math.max(0, document.lineCount - 1), 0)] : [],
  };

  const uriFor = (side: "base" | "head", branch: string, docRef: string) =>
    vscode.Uri.from({ scheme: SCHEME, path: `/${side}/${docRef}`, query: branch });

  const openReview = async (arg?: ChangesetArg) => {
    const client = getClient();
    const branch = argBranch(arg) ?? (await pickChangeset(client));
    if (!branch) {
      return;
    }
    try {
      const [diff, comments] = await Promise.all([client.diff(branch), client.listComments(branch)]);
      const entities = diff.entities;
      const docRefs = unique(entities.map((e) => e.docRef).filter((d): d is string => Boolean(d)));
      if (docRefs.length === 0) {
        vscode.window.showInformationMessage(
          `Cusp: ${branch} has no spec-level changes to diff${comments.length ? " (comments exist but aren't anchored to a spec)" : ""}.`,
        );
        return;
      }
      const refBySubject = new Map(entities.map((e) => [e.subjectRef, e.docRef]));
      disposeThreadsFor(anchors, branch);
      for (let i = 0; i < docRefs.length; i++) {
        await openDocDiff(client, controller, content, heads, anchors, onDidChange, uriFor, {
          branch,
          base: diff.base,
          head: diff.head,
          docRef: docRefs[i],
          comments,
          refBySubject,
          isFirst: i === 0, // changeset-level comments attach to the first diff
        });
      }
    } catch (err) {
      vscode.window.showErrorMessage(`Cusp: failed to open review — ${messageOf(err)}`);
    }
  };

  const reply = async (r: vscode.CommentReply) => {
    const thread = r.thread;
    let anchor = anchors.get(thread) ?? inferAnchor(thread, heads);
    anchors.set(thread, anchor);
    try {
      const input: AddCommentInput = { branch: anchor.branch, body: r.text };
      if (anchor.rootCommentId) {
        input.reply = anchor.rootCommentId;
      } else if (anchor.subject) {
        input.subject = anchor.subject;
      }
      const created = await getClient().addComment(input);
      anchor.rootCommentId ??= created.id;
      thread.comments = [...thread.comments, toVsComment(created)];
      thread.contextValue = "unresolved";
      thread.collapsibleState = vscode.CommentThreadCollapsibleState.Expanded;
    } catch (err) {
      vscode.window.showErrorMessage(`Cusp: failed to add comment — ${messageOf(err)}`);
    }
  };

  const setResolved = (resolved: boolean) => async (thread: vscode.CommentThread) => {
    const anchor = anchors.get(thread);
    if (!anchor?.rootCommentId) {
      return;
    }
    try {
      await getClient().resolveComment(anchor.rootCommentId, resolved);
      thread.contextValue = resolved ? "resolved" : "unresolved";
      thread.collapsibleState = resolved
        ? vscode.CommentThreadCollapsibleState.Collapsed
        : vscode.CommentThreadCollapsibleState.Expanded;
    } catch (err) {
      vscode.window.showErrorMessage(`Cusp: failed to update thread — ${messageOf(err)}`);
    }
  };

  const setVerdict = async (arg?: ChangesetArg) => {
    const client = getClient();
    const branch = argBranch(arg) ?? (await pickChangeset(client));
    if (!branch) {
      return;
    }
    const verdict = await vscode.window.showQuickPick(
      [
        { label: "approve", description: "→ approved" },
        { label: "request_changes", description: "→ changes_requested" },
        { label: "deny", description: "→ denied" },
      ],
      { placeHolder: `Verdict for ${branch}` },
    );
    if (!verdict) {
      return;
    }
    const summary = await vscode.window.showInputBox({ prompt: "Optional review summary" });
    if (summary === undefined) {
      return; // cancelled
    }
    try {
      await client.setReview(branch, verdict.label as Verdict, summary || undefined);
      vscode.window.showInformationMessage(`Cusp: ${verdict.label} on ${branch}`);
      void vscode.commands.executeCommand("cusp.refreshChangesets");
    } catch (err) {
      vscode.window.showErrorMessage(`Cusp: failed to set verdict — ${messageOf(err)}`);
    }
  };

  return vscode.Disposable.from(
    controller,
    onDidChange,
    vscode.workspace.registerTextDocumentContentProvider(SCHEME, contentProvider),
    vscode.commands.registerCommand("cusp.reviewChangeset", openReview),
    vscode.commands.registerCommand("cusp.setVerdict", setVerdict),
    vscode.commands.registerCommand("cusp.review.reply", reply),
    vscode.commands.registerCommand("cusp.review.resolveThread", setResolved(true)),
    vscode.commands.registerCommand("cusp.review.reopenThread", setResolved(false)),
  );
}

interface OpenDocArgs {
  branch: string;
  base: string; // exact merge-base ref for the base side
  head: string; // exact head ref (branch while live, else its commit) for the head side
  docRef: string;
  comments: Comment[];
  refBySubject: Map<string, string>;
  isFirst: boolean;
}

async function openDocDiff(
  client: CuspClient,
  controller: vscode.CommentController,
  content: Map<string, string>,
  heads: Map<string, HeadDoc>,
  anchors: Map<vscode.CommentThread, Anchor>,
  onDidChange: vscode.EventEmitter<vscode.Uri>,
  uriFor: (side: "base" | "head", branch: string, docRef: string) => vscode.Uri,
  a: OpenDocArgs,
): Promise<void> {
  const baseUri = uriFor("base", a.branch, a.docRef);
  const headUri = uriFor("head", a.branch, a.docRef);
  const [baseMd, headMd] = await Promise.all([
    client.renderDocMarkdown(a.docRef, a.base).catch(() => "(new in this changeset)\n"),
    client.renderDocMarkdown(a.docRef, a.head),
  ]);
  content.set(baseUri.toString(), baseMd);
  content.set(headUri.toString(), headMd);
  onDidChange.fire(baseUri);
  onDidChange.fire(headUri);

  const keyLine = anchorLines(headMd);
  // An entity doc has no per-requirement anchors, so any comment on it anchors to the whole entity.
  const docSubject = a.docRef.startsWith("entity:") ? subjectToken(a.docRef) : undefined;
  heads.set(headUri.toString(), { branch: a.branch, docRef: a.docRef, keyLine, docSubject });

  await vscode.commands.executeCommand(
    "vscode.diff",
    baseUri,
    headUri,
    `${displayRef(a.docRef)}: base ↔ ${a.branch}`,
    { preview: false } as vscode.TextDocumentShowOptions,
  );

  // Seed comment threads for comments that belong to this doc (or, for changeset-level comments,
  // onto the first diff opened).
  const roots = a.comments.filter((c) => !c.parentId);
  for (const root of roots) {
    const belongsHere = root.subjectRef
      ? a.refBySubject.get(root.subjectRef) === a.docRef
      : a.isFirst;
    if (!belongsHere) {
      continue;
    }
    const line = root.subjectRef ? keyLine.get(frKeyOf(root.subjectRef)) ?? 0 : 0;
    const chain = [root, ...descendants(root.id, a.comments)];
    const thread = controller.createCommentThread(headUri, new vscode.Range(line, 0, line, 0), chain.map(toVsComment));
    thread.label = root.subjectRef ? `${root.subjectRef}${root.locator ? "@" + root.locator : ""}` : "changeset";
    thread.contextValue = root.resolved ? "resolved" : "unresolved";
    thread.collapsibleState = root.resolved
      ? vscode.CommentThreadCollapsibleState.Collapsed
      : vscode.CommentThreadCollapsibleState.Expanded;
    anchors.set(thread, {
      branch: a.branch,
      subject: root.subjectRef ? subjectToken(root.subjectRef) : undefined,
      rootCommentId: root.id,
    });
  }
}

// anchorLines maps each `^fr-key` block anchor in the rendered markdown to its line (fr-key
// upper-cased), so a comment on a requirement lands on that requirement's line.
function anchorLines(markdown: string): Map<string, number> {
  const map = new Map<string, number>();
  const lines = markdown.split("\n");
  for (let i = 0; i < lines.length; i++) {
    const m = lines[i].match(/\^([a-z0-9][a-z0-9-]*)\s*$/);
    if (m) {
      map.set(m[1].toUpperCase(), i);
    }
  }
  return map;
}

// inferAnchor derives a subject for a thread the user opened on a bare line, from the nearest
// anchored requirement at or above that line in the head doc.
function inferAnchor(thread: vscode.CommentThread, heads: Map<string, HeadDoc>): Anchor {
  const head = heads.get(thread.uri.toString());
  if (!head) {
    return { branch: thread.uri.query || thread.uri.path.replace(/^\/head\//, "") };
  }
  const line = thread.range?.start.line ?? 0;
  let bestKey: string | undefined;
  let bestLine = -1;
  for (const [key, l] of head.keyLine) {
    if (l <= line && l > bestLine) {
      bestKey = key;
      bestLine = l;
    }
  }
  // A requirement anchor above the line wins; otherwise fall back to the doc-level subject (an
  // entity doc), else the comment is changeset-level.
  return { branch: head.branch, subject: bestKey ? `REQ:${bestKey}` : head.docSubject };
}

// descendants returns every comment under rootId (any depth), in listing order.
function descendants(rootId: string, comments: Comment[]): Comment[] {
  const out: Comment[] = [];
  const walk = (parent: string) => {
    for (const c of comments.filter((x) => x.parentId === parent)) {
      out.push(c);
      walk(c.id);
    }
  };
  walk(rootId);
  return out;
}

function toVsComment(c: Comment): vscode.Comment {
  return {
    body: new vscode.MarkdownString(c.body),
    mode: vscode.CommentMode.Preview,
    author: { name: c.authorHandle || "unknown" },
    contextValue: c.id,
    timestamp: c.createdAt ? new Date(c.createdAt) : undefined,
  };
}

async function pickChangeset(client: CuspClient): Promise<string | undefined> {
  const reviewable = (await client.listChangesets()).filter((c) => isReviewable(c.status));
  if (reviewable.length === 0) {
    vscode.window.showInformationMessage("Cusp: no changesets to review.");
    return undefined;
  }
  const pick = await vscode.window.showQuickPick(
    reviewable.map((c) => ({ label: c.branch, description: `${c.status} — ${c.title}` })),
    { placeHolder: "Changeset to review" },
  );
  return pick?.label;
}

function disposeThreadsFor(anchors: Map<vscode.CommentThread, Anchor>, branch: string): void {
  for (const [thread, a] of [...anchors]) {
    if (a.branch === branch) {
      thread.dispose();
      anchors.delete(thread);
    }
  }
}

// "requirement:INV-FR-001" → "INV-FR-001"
function frKeyOf(subjectRef: string): string {
  return subjectRef.split(":").slice(1).join(":").toUpperCase();
}

// "requirement:INV-FR-001" → the `cusp comment --subject` token "REQ:INV-FR-001". Maps the
// subject/doc types that have a ref token; returns undefined for types the CLI can't resolve from
// a token (user_story/test_case/deliverable) — those comments stay changeset-level on a new thread.
function subjectToken(subjectRef: string): string | undefined {
  const [type, ...rest] = subjectRef.split(":");
  const key = rest.join(":");
  const tag: Record<string, string> = { requirement: "REQ", spec: "SPEC", entity: "ENTITY" };
  return tag[type] ? `${tag[type]}:${key}` : undefined;
}

// displayRef strips the "spec:"/"entity:" discriminator from a docRef for a human title.
function displayRef(docRef: string): string {
  return docRef.replace(/^(spec|entity):/, "");
}

function isHeadUri(uri: vscode.Uri): boolean {
  return uri.scheme === SCHEME && uri.path.startsWith("/head/");
}

function unique<T>(xs: T[]): T[] {
  return [...new Set(xs)];
}

function messageOf(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
