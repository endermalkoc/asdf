import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { Changeset, CuspClient } from "./client";

const execFileAsync = promisify(execFile);

/** Raw shape of one `cusp changeset ls --json` row (Go struct field names). */
interface RawChangeset {
  Branch: string;
  Title: string;
  Status: string;
}

/**
 * Talks to a Cusp workspace by running the `cusp` CLI as a subprocess and
 * parsing its `--json` output. This runs wherever the VS Code *workspace*
 * extension host runs — locally, inside WSL, or on a Remote-SSH / dev-container
 * host — i.e. always co-located with the `cusp` binary and the `.cusp` data, so
 * the spawn is always a local exec from the extension's point of view.
 */
export class CliCuspClient implements CuspClient {
  constructor(
    private readonly cliPath: string,
    private readonly cwd: string,
  ) {}

  private async runJSON(args: string[]): Promise<unknown> {
    // `--json` is a persistent flag, valid on every subcommand.
    const { stdout } = await execFileAsync(this.cliPath, [...args, "--json"], {
      cwd: this.cwd,
      maxBuffer: 32 * 1024 * 1024,
    });
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
}
