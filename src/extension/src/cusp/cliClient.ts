import { execFile } from "node:child_process";
import { promisify } from "node:util";
import * as os from "node:os";
import * as path from "node:path";
import * as fs from "node:fs";
import { Changeset, CuspClient, DomainNode } from "./client";

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

  async renderSpecHtml(specRef: string): Promise<string> {
    // Raw HTML on stdout (not a --json envelope).
    return this.exec(["spec", "render", specRef, "--format", "html"]);
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
