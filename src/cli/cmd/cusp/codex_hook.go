package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// `cusp codex-hook <event>` is the hidden shim `cusp setup codex` wires into .codex/hooks.json.
// Codex invokes it per hook event with a JSON payload on stdin; it responds on stdout with the
// Codex hook envelope, injecting `cusp prime` context where appropriate:
//
//   - SessionStart      → inject prime
//   - PostCompact       → drop a re-prime marker (compaction dropped the injected context)
//   - UserPromptSubmit  → if the marker is set, inject prime once and clear it
//   - PreCompact / else → passthrough ({"continue":true})
//
// This mirrors beads' codex-hook: SessionStart can't cover post-compaction re-priming, so the
// marker + next-prompt injection restores context after a compaction.

var codexHookCmd = &cobra.Command{
	Use:    "codex-hook <event>",
	Short:  "Internal: Codex hook shim (emits cusp prime context)",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		event := args[0]
		// Drain Codex's stdin payload so it never blocks on a full pipe.
		_, _ = io.Copy(io.Discard, os.Stdin)

		ctx := cmd.Context()
		inject := func() error {
			content, _ := buildPrime(ctx)
			return emitCodexHook(event, content)
		}
		switch event {
		case "SessionStart":
			return inject()
		case "PostCompact":
			_ = os.WriteFile(codexReprimeMarker(), []byte("1"), 0o644)
			return emitCodexHook(event, "")
		case "UserPromptSubmit":
			if _, err := os.Stat(codexReprimeMarker()); err == nil {
				_ = os.Remove(codexReprimeMarker())
				return inject()
			}
			return emitCodexHook(event, "")
		default:
			return emitCodexHook(event, "")
		}
	},
}

// emitCodexHook writes the Codex hook response: always {"continue":true}, plus additionalContext
// when there is content to inject.
func emitCodexHook(event, content string) error {
	out := map[string]any{"continue": true}
	if strings.TrimSpace(content) != "" {
		out["hookSpecificOutput"] = map[string]any{
			"hookEventName":     event,
			"additionalContext": content,
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

// codexReprimeMarker is a machine-local flag file set on PostCompact and consumed by the next
// UserPromptSubmit, so prime is re-injected exactly once after a compaction.
func codexReprimeMarker() string {
	return filepath.Join(os.TempDir(), "cusp-codex-reprime")
}

func init() {
	rootCmd.AddCommand(codexHookCmd)
}
