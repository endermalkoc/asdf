// The transport-agnostic contract between the extension UI and a Cusp workspace.
//
// The UI talks ONLY to this interface. Today it is implemented by shelling out
// to the `cusp` CLI (see cliClient.ts); when `cusp serve --mcp` lands it can be
// re-implemented over MCP without touching any view code — same methods, same
// shapes, different transport. Keep this file free of `vscode` and `child_process`
// imports so the boundary stays clean.

export interface Changeset {
  /** The Dolt branch holding the proposed change (the changeset's stable id). */
  branch: string;
  title: string;
  /** draft | open | changes_requested | approved | denied | merged | closed */
  status: string;
}

export interface CuspClient {
  /** Mirrors `cusp changeset ls`. */
  listChangesets(): Promise<Changeset[]>;

  // Next slices, as the review surface grows (all already CLI/MCP-shaped):
  //   diff(branch): Promise<EntityDiff[]>          — `cusp changeset diff`
  //   listComments(branch): Promise<Comment[]>     — (needs Step 1: `cusp comment ls`)
  //   addComment(input): Promise<Comment>          — (needs Step 1: `cusp comment add`)
  //   setReview(branch, verdict): Promise<void>    — (needs Step 1: `cusp review`)
}
