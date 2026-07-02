import { execFile } from "node:child_process";
import { promisify } from "node:util";
import * as os from "node:os";
import * as path from "node:path";
import * as fs from "node:fs";
import {
  AddCommentInput,
  Changeset,
  Comment,
  CuspClient,
  DomainNode,
  EntityDiff,
  EntityTreeNode,
  Verdict,
} from "./client";

const execFileAsync = promisify(execFile);

/** Raw shape of one `cusp changeset ls --json` row (Go struct field names). */
interface RawChangeset {
  Branch: string;
  Title: string;
  Status: string;
}

function expandHome(p: string): string {
  return p.startsWith("~/") ? path.join(os.homedir(), p.slice(2)) : p;
}

/**
 * Directories a `cusp` binary commonly lands in but that a GUI-launched VS Code's
 * PATH often omits (the Go install dir, Homebrew, /usr/local), followed by the
 * inherited PATH. A terminal-launched VS Code already has these; this just makes
 * the desktop-launcher case work too.
 */
function searchDirs(): string[] {
  const home = os.homedir();
  const extra = [
    process.env.GOBIN,
    process.env.GOPATH ? path.join(process.env.GOPATH, "bin") : undefined,
    path.join(home, "go", "bin"),
    "/usr/local/bin",
    "/opt/homebrew/bin",
    "/home/linuxbrew/.linuxbrew/bin",
  ].filter((d): d is string => Boolean(d));
  const inherited = (process.env.PATH || "").split(path.delimiter).filter(Boolean);
  return [...extra, ...inherited];
}

/**
 * Resolve cusp.cliPath to an absolute executable. An explicit path (containing a
 * separator, or `~/`) is honored as-is; a bare command name is searched across
 * searchDirs(). Falls back to the bare name so execFile still ENOENTs (and we
 * report a helpful message) when nothing is found.
 */
function locate(cliPath: string): string {
  const p = expandHome(cliPath);
  if (p.includes(path.sep)) {
    return p;
  }
  for (const dir of searchDirs()) {
    const candidate = path.join(dir, p);
    try {
      fs.accessSync(candidate, fs.constants.X_OK);
      return candidate;
    } catch {
      /* not in this dir — keep looking */
    }
  }
  return p;
}

/**
 * Talks to a Cusp workspace by running the `cusp` CLI and parsing its `--json`
 * output. Runs wherever the VS Code *workspace* extension host runs — locally,
 * in WSL, or on a Remote-SSH / dev-container host — always co-located with the
 * binary and the `.cusp` data.
 */
export class CliCuspClient implements CuspClient {
  private readonly cli: string;

  constructor(cliPath: string, private readonly cwd: string) {
    this.cli = locate(cliPath);
  }

  private async exec(args: string[]): Promise<string> {
    try {
      const { stdout } = await execFileAsync(this.cli, args, {
        cwd: this.cwd,
        maxBuffer: 32 * 1024 * 1024,
      });
      return stdout;
    } catch (err) {
      throw describe(err, this.cli);
    }
  }

  private async runJSON(args: string[]): Promise<unknown> {
    // `--json` is a persistent flag, valid on every subcommand.
    const stdout = await this.exec([...args, "--json"]);
    const trimmed = stdout.trim();
    return trimmed.length > 0 ? JSON.parse(trimmed) : null;
  }

  async listChangesets(): Promise<Changeset[]> {
    const raw = (await this.runJSON(["changeset", "ls"])) as RawChangeset[] | null;
    if (!raw) {
      return [];
    }
    return raw.map((r) => ({ branch: r.Branch, title: r.Title, status: r.Status }));
  }

  async requirementsTree(): Promise<DomainNode[]> {
    const raw = (await this.runJSON(["req", "tree"])) as DomainNode[] | null;
    return raw ?? [];
  }

  async entitiesTree(): Promise<EntityTreeNode[]> {
    const raw = (await this.runJSON(["entity", "tree"])) as EntityTreeNode[] | null;
    return raw ?? [];
  }

  async renderDocHtml(docPath: string): Promise<string> {
    // Raw HTML on stdout (not a --json envelope). Entity docs live under entities/.
    const args = docPath.startsWith("entities/")
      ? ["entity", "render", docPath, "--format", "html"]
      : ["spec", "render", docPath, "--format", "html"];
    return this.exec(args);
  }

  async renderDocMarkdown(docRef: string, branch: string): Promise<string> {
    // docRef is a self-describing token from `changeset diff --entities` — `spec:<ref>` or
    // `entity:<name>`. `--changeset <branch>` selects the base (main) or head branch.
    const [kind, ...rest] = docRef.split(":");
    const ref = rest.join(":");
    const args =
      kind === "entity"
        ? ["entity", "render", ref, "--format", "md", "--changeset", branch]
        : ["spec", "render", ref, "--format", "md", "--changeset", branch];
    return this.exec(args);
  }

  async diff(branch: string): Promise<EntityDiff[]> {
    const raw = (await this.runJSON(["changeset", "diff", branch, "--entities"])) as EntityDiff[] | null;
    return raw ?? [];
  }

  async listComments(branch: string): Promise<Comment[]> {
    const raw = (await this.runJSON(["comment", "ls", branch])) as Comment[] | null;
    return raw ?? [];
  }

  async addComment(input: AddCommentInput): Promise<Comment> {
    const args = ["comment", "add", input.branch, "--body", input.body];
    if (input.subject) {
      args.push("--subject", input.subject);
    } else if (input.subjectType && input.subjectId) {
      args.push("--subject-type", input.subjectType, "--subject-id", input.subjectId);
    }
    if (input.locator) {
      args.push("--locator", input.locator);
    }
    if (input.reply) {
      args.push("--reply", input.reply);
    }
    return (await this.runJSON(args)) as Comment;
  }

  async resolveComment(id: string, resolved: boolean): Promise<void> {
    await this.runJSON(["comment", resolved ? "resolve" : "reopen", id]);
  }

  async setReview(branch: string, verdict: Verdict, summary?: string): Promise<void> {
    const args = ["review", branch, "--verdict", verdict];
    if (summary) {
      args.push("--summary", summary);
    }
    await this.runJSON(args);
  }
}

interface ExecError extends Error {
  code?: string | number;
  stderr?: string;
}

/** Turn raw spawn/CLI failures into actionable messages. */
function describe(err: unknown, cli: string): Error {
  const e = err as ExecError;
  if (e?.code === "ENOENT") {
    return new Error(
      `could not find the cusp binary (tried "${cli}"). Install it with \`make install\`, ` +
        `or set the "cusp.cliPath" setting to its absolute path.`,
    );
  }
  // cusp emits a structured error envelope under --json; surface it.
  const stderr = (e?.stderr || "").trim();
  if (stderr) {
    return new Error(stderr);
  }
  return e instanceof Error ? e : new Error(String(err));
}
