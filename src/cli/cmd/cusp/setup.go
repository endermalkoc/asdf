package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/agentsetup"
	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/git"
)

// `cusp setup <recipe>` installs Cusp's agent-integration artifacts into a repo (or the user's
// home with --global): an instruction section in the host's agent file, the agent skill, and a
// SessionStart hook that runs `cusp prime`. Everything is idempotent and reversible (--remove),
// with a dry status report (--check). See internal/agentsetup for the marker engine + templates.

// The command a host's SessionStart hook runs. `cusp` is expected on PATH (the agent host runs it).
const primeHookCommand = "cusp prime --hook-json"

var (
	setupList   bool
	setupPrint  bool
	setupCheck  bool
	setupRemove bool
	setupGlobal bool
)

// setupResult is one artifact touched by an install/check/remove, for output.
type setupResult struct {
	Path   string `json:"path"`
	Action string `json:"action"` // created|updated|unchanged|appended|removed|present|stale|missing|skipped
}

// recipe is a per-host installer.
type recipe struct {
	name    string
	title   string
	install func(base string, global bool) ([]setupResult, error)
	check   func(base string, global bool) ([]setupResult, error)
	remove  func(base string, global bool) ([]setupResult, error)
}

var recipes = map[string]recipe{}

var setupCmd = &cobra.Command{
	Use:   "setup [recipe]",
	Short: "Install Cusp agent integration for an editor/agent (instructions + skill + hook)",
	Long: "Install Cusp's agent-integration artifacts for a coding agent: an instruction section in\n" +
		"its agent file (CLAUDE.md / AGENTS.md), the Cusp skill, and a SessionStart hook that runs\n" +
		"`cusp prime` to inject live workspace state. Idempotent (re-run to update), reversible\n" +
		"(--remove), with a status report (--check).\n\n" +
		"  cusp setup --list            # available recipes\n" +
		"  cusp setup claude            # install for Claude Code (project)\n" +
		"  cusp setup codex --check     # report what's installed / stale\n" +
		"  cusp setup claude --remove   # remove Cusp's artifacts\n" +
		"  cusp setup --print           # print the instruction section",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if setupList {
			printRecipeList()
			return nil
		}
		if setupPrint {
			fmt.Print(agentsetup.SectionBody)
			return nil
		}
		if len(args) == 0 {
			return app.ValidationFailed(fmt.Errorf("name a recipe (see `cusp setup --list`), or use --print"))
		}
		rc, ok := recipes[strings.ToLower(args[0])]
		if !ok {
			return app.ValidationFailed(fmt.Errorf("unknown recipe %q — see `cusp setup --list`", args[0]))
		}
		base, err := setupBase(setupGlobal)
		if err != nil {
			return err
		}
		var (
			results []setupResult
			verb    string
		)
		switch {
		case setupCheck:
			results, err = rc.check(base, setupGlobal)
			verb = "checked"
		case setupRemove:
			results, err = rc.remove(base, setupGlobal)
			verb = "removed"
		default:
			results, err = rc.install(base, setupGlobal)
			verb = "installed"
		}
		if err != nil {
			return err
		}
		if flagJSON {
			emit(results, "")
			return nil
		}
		scope := "project"
		if setupGlobal {
			scope = "global"
		}
		fmt.Printf("%s %s (%s) at %s\n", verb, rc.title, scope, base)
		for _, r := range results {
			fmt.Printf("  %-10s %s\n", r.Action, r.Path)
		}
		return nil
	},
}

// setupBase resolves where artifacts are written: the user's home for --global, else the repo root.
func setupBase(global bool) (string, error) {
	if global {
		return os.UserHomeDir()
	}
	root, err := git.GetMainRepoRoot()
	if err != nil {
		return "", fmt.Errorf("not in a git repository — run `cusp setup` inside your project (or pass --global)")
	}
	return root, nil
}

func printRecipeList() {
	names := make([]string, 0, len(recipes))
	for n := range recipes {
		names = append(names, n)
	}
	sort.Strings(names)
	if flagJSON {
		type rl struct{ Name, Title string }
		var out []rl
		for _, n := range names {
			out = append(out, rl{n, recipes[n].title})
		}
		emit(out, "")
		return
	}
	fmt.Println("Available setup recipes:")
	for _, n := range names {
		fmt.Printf("  %-10s %s\n", n, recipes[n].title)
	}
}

// ---- shared helpers -------------------------------------------------------

// writeFileIfChanged writes content to path (creating parent dirs), reporting whether it created,
// updated, or left the file unchanged.
func writeFileIfChanged(path, content string) (agentsetup.InstallAction, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if existing, err := os.ReadFile(path); err == nil {
		if string(existing) == content {
			return agentsetup.ActionUnchanged, nil
		}
		if werr := os.WriteFile(path, []byte(content), 0o644); werr != nil {
			return "", werr
		}
		return agentsetup.ActionUpdated, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return agentsetup.ActionCreated, nil
}

// rel renders a path relative to base for display, falling back to the absolute path.
func rel(base, path string) string {
	if r, err := filepath.Rel(base, path); err == nil {
		return r
	}
	return path
}

func isCuspPrimeCommand(cmd string) bool {
	return strings.Contains(cmd, "cusp") && strings.Contains(cmd, "prime")
}

// ---- JSON settings helpers (Claude / Gemini style hooks) ------------------

func loadJSONObject(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return m, nil
}

func writeJSONObject(path string, m map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func asObject(m map[string]any, key string) map[string]any {
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return map[string]any{}
}

// upsertSessionStartHook adds `command` as a SessionStart command-hook in a Claude/Gemini-style
// settings.json, sweeping any legacy cusp-prime variants. Returns "added" | "updated" | "unchanged".
func upsertSessionStartHook(path, command string) (string, error) {
	root, err := loadJSONObject(path)
	if err != nil {
		return "", err
	}
	hooks := asObject(root, "hooks")
	list, _ := hooks["SessionStart"].([]any)

	present, swept := false, false
	out := make([]any, 0, len(list)+1)
	for _, entry := range list {
		em, ok := entry.(map[string]any)
		if !ok {
			out = append(out, entry)
			continue
		}
		inner, _ := em["hooks"].([]any)
		keep := make([]any, 0, len(inner))
		for _, h := range inner {
			hm, ok := h.(map[string]any)
			if !ok {
				keep = append(keep, h)
				continue
			}
			cmd, _ := hm["command"].(string)
			switch {
			case cmd == command:
				present = true
				keep = append(keep, h)
			case isCuspPrimeCommand(cmd):
				swept = true // drop a legacy cusp-prime variant
			default:
				keep = append(keep, h)
			}
		}
		if len(keep) > 0 || len(inner) == 0 {
			em["hooks"] = keep
			out = append(out, em)
		}
	}

	if present && !swept {
		return "unchanged", nil
	}
	action := "updated"
	if !present {
		out = append(out, map[string]any{
			"matcher": "",
			"hooks":   []any{map[string]any{"type": "command", "command": command}},
		})
		action = "added"
	}
	hooks["SessionStart"] = out
	root["hooks"] = hooks
	if err := writeJSONObject(path, root); err != nil {
		return "", err
	}
	return action, nil
}

// removeSessionStartHook removes every cusp-prime SessionStart command from settings.json,
// dropping empty entries and the empty hooks key. Returns whether it changed anything.
func removeSessionStartHook(path string) (bool, error) {
	root, err := loadJSONObject(path)
	if err != nil {
		return false, err
	}
	hooks, ok := root["hooks"].(map[string]any)
	if !ok {
		return false, nil
	}
	list, _ := hooks["SessionStart"].([]any)
	changed := false
	out := make([]any, 0, len(list))
	for _, entry := range list {
		em, ok := entry.(map[string]any)
		if !ok {
			out = append(out, entry)
			continue
		}
		inner, _ := em["hooks"].([]any)
		keep := make([]any, 0, len(inner))
		for _, h := range inner {
			if hm, ok := h.(map[string]any); ok {
				if cmd, _ := hm["command"].(string); isCuspPrimeCommand(cmd) {
					changed = true
					continue
				}
			}
			keep = append(keep, h)
		}
		if len(keep) > 0 {
			em["hooks"] = keep
			out = append(out, em)
		}
	}
	if !changed {
		return false, nil
	}
	if len(out) > 0 {
		hooks["SessionStart"] = out
	} else {
		delete(hooks, "SessionStart")
	}
	if len(hooks) == 0 {
		delete(root, "hooks")
	}
	if err := writeJSONObject(path, root); err != nil {
		return false, err
	}
	return true, nil
}

// sessionStartHookStatus reports whether our SessionStart hook command is present in settings.json.
func sessionStartHookStatus(path, command string) (present bool, err error) {
	root, err := loadJSONObject(path)
	if err != nil {
		return false, err
	}
	hooks, ok := root["hooks"].(map[string]any)
	if !ok {
		return false, nil
	}
	list, _ := hooks["SessionStart"].([]any)
	for _, entry := range list {
		em, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		inner, _ := em["hooks"].([]any)
		for _, h := range inner {
			if hm, ok := h.(map[string]any); ok {
				if cmd, _ := hm["command"].(string); cmd == command {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// ---- Claude Code recipe ---------------------------------------------------

func claudeSkillPath(base string) string {
	return filepath.Join(base, ".claude", "skills", "cusp", "SKILL.md")
}
func claudeSettingsPath(base string) string { return filepath.Join(base, ".claude", "settings.json") }
func claudeInstructionsPath(base string, global bool) string {
	if global {
		return filepath.Join(base, ".claude", "CLAUDE.md")
	}
	return filepath.Join(base, "CLAUDE.md")
}

func claudeInstall(base string, global bool) ([]setupResult, error) {
	var out []setupResult

	skill := claudeSkillPath(base)
	act, err := writeFileIfChanged(skill, agentsetup.SkillBody)
	if err != nil {
		return nil, err
	}
	out = append(out, setupResult{rel(base, skill), string(act)})

	instr := claudeInstructionsPath(base, global)
	iact, err := agentsetup.InstallSection(instr, agentsetup.SectionBody)
	if err != nil {
		return nil, err
	}
	out = append(out, setupResult{rel(base, instr), string(iact)})

	settings := claudeSettingsPath(base)
	hact, err := upsertSessionStartHook(settings, primeHookCommand)
	if err != nil {
		return nil, err
	}
	out = append(out, setupResult{rel(base, settings), hact})
	return out, nil
}

func claudeCheck(base string, global bool) ([]setupResult, error) {
	var out []setupResult

	skill := claudeSkillPath(base)
	out = append(out, setupResult{rel(base, skill), fileStatus(skill, agentsetup.SkillBody)})

	instr := claudeInstructionsPath(base, global)
	out = append(out, setupResult{rel(base, instr), sectionStatusLabel(instr, agentsetup.SectionBody)})

	settings := claudeSettingsPath(base)
	present, err := sessionStartHookStatus(settings, primeHookCommand)
	if err != nil {
		return nil, err
	}
	out = append(out, setupResult{rel(base, settings), boolStatus(present, "present", "missing")})
	return out, nil
}

func claudeRemove(base string, global bool) ([]setupResult, error) {
	var out []setupResult

	skill := claudeSkillPath(base)
	out = append(out, setupResult{rel(base, skill), removeFileLabel(skill)})

	instr := claudeInstructionsPath(base, global)
	removed, err := agentsetup.RemoveSection(instr)
	if err != nil {
		return nil, err
	}
	out = append(out, setupResult{rel(base, instr), boolStatus(removed, "removed", "absent")})

	settings := claudeSettingsPath(base)
	rmHook, err := removeSessionStartHook(settings)
	if err != nil {
		return nil, err
	}
	out = append(out, setupResult{rel(base, settings), boolStatus(rmHook, "removed", "absent")})
	return out, nil
}

// ---- status/remove label helpers ------------------------------------------

func fileStatus(path, want string) string {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "missing"
	}
	if err != nil {
		return "error"
	}
	if string(data) == want {
		return "present"
	}
	return "stale"
}

func sectionStatusLabel(path, body string) string {
	present, current, err := agentsetup.SectionStatus(path, body)
	if err != nil {
		return "error"
	}
	switch {
	case !present:
		return "missing"
	case current:
		return "present"
	default:
		return "stale"
	}
}

func removeFileLabel(path string) string {
	err := os.Remove(path)
	if err == nil {
		return "removed"
	}
	if os.IsNotExist(err) {
		return "absent"
	}
	return "error"
}

func boolStatus(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}

func init() {
	setupCmd.Flags().BoolVar(&setupList, "list", false, "list available recipes")
	setupCmd.Flags().BoolVar(&setupPrint, "print", false, "print the instruction section and exit")
	setupCmd.Flags().BoolVar(&setupCheck, "check", false, "report what is installed / stale / missing")
	setupCmd.Flags().BoolVar(&setupRemove, "remove", false, "remove Cusp's artifacts for the recipe")
	setupCmd.Flags().BoolVar(&setupGlobal, "global", false, "target the user's home config instead of the project")

	recipes["claude"] = recipe{
		name: "claude", title: "Claude Code",
		install: claudeInstall, check: claudeCheck, remove: claudeRemove,
	}
	rootCmd.AddCommand(setupCmd)
}
