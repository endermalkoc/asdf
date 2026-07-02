package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/configfile"
	"github.com/endermalkoc/cusp/internal/workspace"
)

// `cusp prime` emits the context an agent needs when a session starts in a Cusp workspace:
// the live state (active/open changesets, review + integrity signals, counts) followed by a
// compact workflow reference. It is the payload the SessionStart hooks installed by `cusp
// setup` inject. Designed to be safe as a universal hook: it stays silent when run outside a
// Cusp workspace, and degrades to the static reference if the database is unreachable.

var primeHookJSON bool

var primeCmd = &cobra.Command{
	Use:   "prime",
	Short: "Emit agent session context — live state + the Cusp workflow (for a SessionStart hook)",
	Long: "Print the context an agent should have when it starts working in this Cusp workspace:\n" +
		"the active/open changesets, unresolved review comments, integrity status, and headline\n" +
		"counts, then a compact workflow reference. `cusp setup` wires this into each agent's\n" +
		"SessionStart hook (via --hook-json).\n\n" +
		"  cusp prime               # Markdown to stdout\n" +
		"  cusp prime --hook-json   # wrapped in the SessionStart hook envelope\n" +
		"  cusp prime --json        # the structured live state\n\n" +
		"Silent when run outside a Cusp workspace (safe as a universal hook). A repo-local\n" +
		"`.cusp/PRIME.md` overrides the generated payload verbatim.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		content, state := buildPrime(ctx)
		if strings.TrimSpace(content) == "" {
			return nil // no workspace / nothing to say — stay silent
		}
		switch {
		case primeHookJSON:
			return emitSessionStartHook(content)
		case flagJSON:
			emit(state, "")
			return nil
		default:
			fmt.Print(content)
			return nil
		}
	},
}

// buildPrime assembles the prime payload and (when available) the structured state behind it.
// Returns "" when there is no Cusp workspace here.
func buildPrime(ctx context.Context) (string, *app.PrimeState) {
	cuspDir, err := workspace.ResolveCuspDir()
	if err != nil {
		return "", nil // not in a git repo
	}
	if _, err := os.Stat(configfile.ConfigPath(cuspDir)); err != nil {
		return "", nil // no Cusp workspace at this root
	}
	// A repo-local override replaces the generated payload entirely.
	if b, err := os.ReadFile(filepath.Join(cuspDir, "PRIME.md")); err == nil {
		return string(b), nil
	}
	// Live state is best-effort: if the DB can't be reached, still emit the workflow reference
	// so the hook is useful (and never fails the session).
	var stateSection string
	var state *app.PrimeState
	if ws, err := connect(ctx); err == nil {
		state = app.GatherPrime(ctx, ws)
		stateSection = renderPrimeState(state)
		_ = ws.Close()
	}
	return stateSection + primeWorkflowRef, state
}

// emitSessionStartHook prints the additionalContext wrapped in the SessionStart hook envelope
// that Claude Code / Gemini / the Codex shim consume on stdout.
func emitSessionStartHook(content string) error {
	env := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     "SessionStart",
			"additionalContext": content,
		},
	}
	b, err := json.Marshal(env)
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

// renderPrimeState renders the live workspace snapshot as a Markdown section.
func renderPrimeState(st *app.PrimeState) string {
	var b strings.Builder
	b.WriteString("# Cusp workspace — current state\n\n")
	if st.ActiveChangeset != "" {
		b.WriteString("- **Active changeset:** `" + st.ActiveChangeset + "`\n")
	} else {
		b.WriteString("- **Active changeset:** none — edits commit to `main`; start one with `cusp changeset start \"<title>\"`\n")
	}
	if len(st.OpenChangesets) > 0 {
		fmt.Fprintf(&b, "- **Open changesets:** %d\n", len(st.OpenChangesets))
		for _, c := range st.OpenChangesets {
			marker := " "
			if c.Active {
				marker = "*"
			}
			title := c.Title
			if title == "" {
				title = "(untitled)"
			}
			fmt.Fprintf(&b, "    - %s `%s`  %s  %s\n", marker, c.Branch, c.Status, title)
		}
	} else {
		b.WriteString("- **Open changesets:** none\n")
	}
	fmt.Fprintf(&b, "- **Unresolved review comments:** %d\n", st.UnresolvedComments)
	if st.CheckFindings == 0 {
		b.WriteString("- **Integrity (`cusp check`):** clean\n")
	} else {
		fmt.Fprintf(&b, "- **Integrity (`cusp check`):** %d issue(s) — run `cusp check`\n", st.CheckFindings)
	}
	if len(st.Stats) > 0 {
		parts := make([]string, 0, len(st.Stats))
		for _, s := range st.Stats {
			parts = append(parts, fmt.Sprintf("%s %d", s.Kind, s.Count))
		}
		b.WriteString("- **Counts:** " + strings.Join(parts, " · ") + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

// primeWorkflowRef is the static how-to-work-in-this-workspace reference appended to every
// prime payload (kept compact — it rides on every session start).
const primeWorkflowRef = "# Cusp — Delivery Lifecycle Graph\n\n" +
	"This repository uses **Cusp**: a Dolt-backed, version-controlled source of truth for the\n" +
	"project's specs, requirements, tests, and plans. Drive it with the `cusp` CLI.\n\n" +
	"## Invariants (do not violate)\n" +
	"- The **Dolt database is the single source of truth.** Generated Markdown/HTML (under `.cusp/`\n" +
	"  or a `cusp generate` output dir) are **build artifacts** — never hand-edit them; change\n" +
	"  content through `cusp` and regenerate.\n" +
	"- **All writes go through `cusp`** (or its MCP server). You may *read* generated Markdown for\n" +
	"  speed, but it is never a write path.\n\n" +
	"## Changeset workflow (the PR model)\n" +
	"Edits are grouped into a **changeset** (a Dolt branch), reviewed, then merged:\n" +
	"1. `cusp changeset start \"<title>\"` — begin (it becomes the active changeset)\n" +
	"2. edit — `cusp req add|edit`, `cusp spec add`, `cusp entity …`, `cusp test …`, `cusp edge add`\n" +
	"3. `cusp changeset diff` — review the pending change\n" +
	"4. `cusp changeset submit` — mark ready for review\n" +
	"5. `cusp review --verdict approve|request_changes|deny` · `cusp comment add` — review\n" +
	"6. `cusp changeset merge` — land it on `main`\n\n" +
	"## Reading\n" +
	"- `cusp req tree` · `cusp search <text>` · `cusp stats` · `cusp impact <TYPE:key>`\n" +
	"- `cusp check` — integrity gate (dangling refs, edge cycles); run it before submitting.\n\n" +
	"Run `cusp <command> --help` for details. Prefer `--json` for machine-readable output.\n"

func init() {
	primeCmd.Flags().BoolVar(&primeHookJSON, "hook-json", false,
		"wrap the payload in the SessionStart hook envelope (for agent hooks)")
	rootCmd.AddCommand(primeCmd)
}
