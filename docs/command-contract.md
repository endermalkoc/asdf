# Command contract

The single workflow **every** ADLG CLI command implements, so behavior is uniform and no
command drifts. This turns the cross-cutting concerns listed as gaps in [ROADMAP.md](ROADMAP.md)
("Domain-layer gaps") into a per-command standard.

## Enforcement principle: one pipeline, not a per-command checklist

A checklist that each command author must remember is how commands drift. Instead, the
cross-cutting steps live in **one shared wrapper**; a command supplies only its inputs,
validation, and the actual row writes. A command **physically cannot** skip commit, attribution,
or transaction handling because the wrapper owns them.

```
// every mutating command body is:
return mutate(cmd, MutateOpts{Summary: "add requirement ENR-FR-002"}, func(ctx context.Context, w *Write) error {
    // 1. validate inputs (fail before any write)
    // 2. mint ids, write rows via w.Tx (one transaction)
    return nil
})
// mutate() owns: connect → resolve write target (changeset/main) → BEGIN tx →
//                [body] → commit-as-Dolt-commit(actor,msg) → output → error/exit mapping
```

This mirrors beads' `HookFiringStore` decorator + `withRetryTx` + uniform command wrapper —
the reason every `bd` verb behaves consistently.

## Mutating commands (`add` / `edit` / `delete` / `link` / `set`) — the contract

Every mutating command MUST, in order:

1. **Connect** through resolved config — DSN / DB location from `internal/config`, never
   hardcoded; via the managed connection (the `adlg init`-created DB / `doltserver`). *(gap #1, #9)*
2. **Resolve the write target** — the ambient active changeset or `--changeset <name>` (its Dolt
   branch); otherwise `main` (auto-commit). *(changeset model — decisions.md)*
3. **Validate inputs first** — enum values against the allowed set, required fields, business
   constraints, and existence/type of referenced entities. Validation runs **on the resolved target
   branch** (after the branch is selected), so existence/ref checks see rows staged in the active
   changeset, not stale `main`. Fail with a clear message + nonzero exit **before any write**. *(gap #4, #7)*
4. **Mint ids** — `ids.New()` for authored rows, `ids.Rel()` for relationship rows. *(done)*
5. **Write atomically** — all rows for one logical change (entity + its junctions/edges) in a
   single transaction; roll back on any error. No half-applied writes. *(gap #3)*
6. **Attribute + timestamp** — set `created_at`/`updated_at` (UTC) and actor/owner from identity. *(gap #5)*
7. **Commit** — record a Dolt commit with actor + message on the target (changeset branch or
   `main`), or accumulate into the changeset's working set per the granularity policy. Never
   leave a write uncommitted in `main`'s working set. *(gap #2)*
8. **Be concurrency-safe** — atomic allocation for sequential numbers; retry on serialization
   errors (1213/1205). *(gap #6, #8)*
9. **Output uniformly** — human text by default; `--json` emits a schema-versioned envelope;
   `--dry-run` previews the change without writing. *(gap #9)*
10. **Map errors + exit codes** — structured, `--json`-aware, with documented exit codes. *(gap #8)*

## Read commands (`ls` / `get` / `show` / `diff`) — the contract

1. **Connect** through resolved config (as above).
2. **Respect the read target** — read from the active/`--changeset` branch if set, else `main`.
   Implemented via `app.Reader` (pin a connection + check out the resolved branch), since Dolt
   branch state is connection-scoped and the shared pool (`ws.DB()`) sits on `main`. Reads whose
   rows always live on `main` (e.g. `changeset ls` → `rev_changeset`) read the pool directly.
3. **Output uniformly** — text default, `--json` envelope.
4. **Map errors + exit codes** — structured, `--json`-aware.

(Reads never write, commit, or mutate working-set state.)

## `--dry-run` and exit codes

Every mutating command accepts the global `--dry-run` flag: the body runs and validates inside a
transaction, then rolls back — nothing is committed (the CLI prints a `[dry-run] … no changes were
committed` note). It is injected once by the `runMutate` CLI wrapper (`cmd/adlg/root.go`), not per
command, over `app.Mutate`'s existing `DryRun`.

Failures map to documented **exit codes** (and, under `--json`, a structured error envelope
`{"error":{"code","category","message"}}` on stdout):

| code | category       | when |
|------|----------------|------|
| 0    | —              | success |
| 1    | `error`        | generic / uncategorized failure |
| 2    | `validation`   | invalid enum / missing required field (`app.ValidateEnum`/`ValidateRequired`/strict soft) |
| 3    | `not_found`    | a named entity does not exist (`app.NotFound`/`NotFoundErr`) |
| 4    | `dangling_ref` | an inline `[[TYPE:key]]` cross-reference does not resolve (`app.DanglingError`) |

Errors are tagged at their source via `app.CodedError`; `Execute` (`cmd/adlg/root.go`) maps the
code and renders the envelope. Uncategorized errors stay exit 1.

## Current status (the slice vs the contract)

The wrapper exists (`internal/app.Mutate`) and the slice now routes through it. `domain`/`spec`/
`req`/`edge` `add` satisfy the mutating-command contract — managed connect, changeset/main
target resolution, validation, transaction, mint, attribution + timestamps, and a real Dolt
commit with actor+message. `adlg init` bootstraps the workspace; `adlg changeset
start/diff/submit/merge/abandon/ls` provide the PR flow. Reads (`ls`) follow the read contract.

Structured error→exit-code mapping and `--dry-run` are now wired (above); graph integrity (edge
cycle detection / polymorphic endpoint checks) is built; hooks are not. New commands inherit the
full workflow by construction — this doc remains the review checklist.

See also: [decisions.md — Changeset model](entities/decisions.md), [ROADMAP.md](ROADMAP.md) gaps,
[ARCHITECTURE.md](ARCHITECTURE.md).
