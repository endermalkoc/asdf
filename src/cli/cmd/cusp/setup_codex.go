package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/endermalkoc/cusp/internal/agentsetup"
)

// The Codex recipe. Codex reads AGENTS.md and supports native hooks: `cusp setup codex` writes
// the skill under .agents/skills/cusp/, enables the hooks feature in .codex/config.toml, writes
// .codex/hooks.json routing four events through the hidden `cusp codex-hook <event>` shim (see
// codex_hook.go), and injects the managed section into AGENTS.md.

// codexHookPrefix identifies our managed entries in hooks.json (so re-runs replace, not duplicate).
const codexHookPrefix = "cusp codex-hook "

func codexHookCommand(event string) string { return codexHookPrefix + event }

// codexHookEvents are the Codex hook events we manage, with their matchers. SessionStart primes;
// PostCompact + UserPromptSubmit re-prime after a compaction (see the shim); PreCompact is a
// passthrough placeholder.
var codexHookEvents = []struct{ Event, Matcher string }{
	{"SessionStart", "startup|resume|clear"},
	{"UserPromptSubmit", ""},
	{"PreCompact", "manual|auto"},
	{"PostCompact", "manual|auto"},
}

func codexSkillPath(base string) string {
	return filepath.Join(base, ".agents", "skills", "cusp", "SKILL.md")
}
func codexSkillMetaPath(base string) string {
	return filepath.Join(base, ".agents", "skills", "cusp", "agents", "openai.yaml")
}
func codexConfigPath(base string) string { return filepath.Join(base, ".codex", "config.toml") }
func codexHooksPath(base string) string  { return filepath.Join(base, ".codex", "hooks.json") }
func codexInstructionsPath(base string, global bool) string {
	if global {
		return filepath.Join(base, ".codex", "AGENTS.md")
	}
	return filepath.Join(base, "AGENTS.md")
}

func codexInstall(base string, global bool) ([]setupResult, error) {
	var out []setupResult

	skill := codexSkillPath(base)
	act, err := writeFileIfChanged(skill, agentsetup.SkillBody)
	if err != nil {
		return nil, err
	}
	out = append(out, setupResult{rel(base, skill), string(act)})

	meta := codexSkillMetaPath(base)
	act, err = writeFileIfChanged(meta, agentsetup.OpenAISkillMeta)
	if err != nil {
		return nil, err
	}
	out = append(out, setupResult{rel(base, meta), string(act)})

	cfg := codexConfigPath(base)
	cact, err := enableCodexHooksFeature(cfg)
	if err != nil {
		return nil, err
	}
	out = append(out, setupResult{rel(base, cfg), cact})

	hooks := codexHooksPath(base)
	hact, err := upsertCodexHooks(hooks)
	if err != nil {
		return nil, err
	}
	out = append(out, setupResult{rel(base, hooks), hact})

	instr := codexInstructionsPath(base, global)
	iact, err := agentsetup.InstallSection(instr, agentsetup.SectionBody)
	if err != nil {
		return nil, err
	}
	out = append(out, setupResult{rel(base, instr), string(iact)})
	return out, nil
}

func codexCheck(base string, global bool) ([]setupResult, error) {
	var out []setupResult
	out = append(out, setupResult{rel(base, codexSkillPath(base)), fileStatus(codexSkillPath(base), agentsetup.SkillBody)})
	out = append(out, setupResult{rel(base, codexConfigPath(base)), codexFeatureStatus(codexConfigPath(base))})
	out = append(out, setupResult{rel(base, codexHooksPath(base)), boolStatus(codexHooksPresent(codexHooksPath(base)), "present", "missing")})
	instr := codexInstructionsPath(base, global)
	out = append(out, setupResult{rel(base, instr), sectionStatusLabel(instr, agentsetup.SectionBody)})
	return out, nil
}

func codexRemove(base string, global bool) ([]setupResult, error) {
	var out []setupResult

	skillDir := filepath.Dir(codexSkillPath(base))
	act := "absent"
	if _, err := os.Stat(skillDir); err == nil {
		if err := os.RemoveAll(skillDir); err != nil {
			return nil, err
		}
		act = "removed"
	}
	out = append(out, setupResult{rel(base, skillDir), act})

	// Leave the config.toml hooks feature flag alone — it's harmless and may be wanted for
	// other hooks; we only remove our managed hooks.json entries.
	out = append(out, setupResult{rel(base, codexConfigPath(base)), "kept"})

	rmHooks, err := removeCodexHooks(codexHooksPath(base))
	if err != nil {
		return nil, err
	}
	out = append(out, setupResult{rel(base, codexHooksPath(base)), boolStatus(rmHooks, "removed", "absent")})

	instr := codexInstructionsPath(base, global)
	removed, err := agentsetup.RemoveSection(instr)
	if err != nil {
		return nil, err
	}
	out = append(out, setupResult{rel(base, instr), boolStatus(removed, "removed", "absent")})
	return out, nil
}

// ---- .codex/config.toml [features] hooks = true ---------------------------

var (
	codexHooksTrueRe = regexp.MustCompile(`(?m)^\s*hooks\s*=\s*true\b`)
	codexHooksAnyRe  = regexp.MustCompile(`(?m)^(\s*hooks\s*=\s*).*$`)
)

// enableCodexHooksFeature ensures `[features] hooks = true` in config.toml (a minimal text upsert
// so an existing config is preserved). Returns the action (added|updated|unchanged).
func enableCodexHooksFeature(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	content := string(data)
	if codexHooksTrueRe.MatchString(content) {
		return "unchanged", nil
	}
	var updated, action string
	switch {
	case codexHooksAnyRe.MatchString(content):
		updated = codexHooksAnyRe.ReplaceAllString(content, "${1}true")
		action = "updated"
	case strings.Contains(content, "[features]"):
		updated = strings.Replace(content, "[features]", "[features]\nhooks = true", 1)
		action = "updated"
	default:
		trimmed := strings.TrimRight(content, "\n")
		if trimmed != "" {
			trimmed += "\n\n"
		}
		updated = trimmed + "[features]\nhooks = true\n"
		action = "added"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return "", err
	}
	return action, nil
}

func codexFeatureStatus(path string) string {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "missing"
	}
	if err != nil {
		return "error"
	}
	if codexHooksTrueRe.Match(data) {
		return "present"
	}
	return "missing"
}

// ---- .codex/hooks.json ----------------------------------------------------

func codexManagedHooks() map[string][]any {
	m := map[string][]any{}
	for _, e := range codexHookEvents {
		entry := map[string]any{
			"hooks": []any{map[string]any{"type": "command", "command": codexHookCommand(e.Event)}},
		}
		if e.Matcher != "" {
			entry["matcher"] = e.Matcher
		}
		m[e.Event] = []any{entry}
	}
	return m
}

func isCodexManagedEntry(entry any) bool {
	em, ok := entry.(map[string]any)
	if !ok {
		return false
	}
	inner, _ := em["hooks"].([]any)
	for _, h := range inner {
		if hm, ok := h.(map[string]any); ok {
			if cmd, _ := hm["command"].(string); strings.HasPrefix(cmd, codexHookPrefix) {
				return true
			}
		}
	}
	return false
}

// upsertCodexHooks writes our managed hook set into hooks.json, preserving any user-defined hooks
// and replacing (not duplicating) our own. Returns installed|unchanged.
func upsertCodexHooks(path string) (string, error) {
	root, err := loadJSONObject(path)
	if err != nil {
		return "", err
	}
	before, _ := json.Marshal(root)
	managed := codexManagedHooks()
	for _, e := range codexHookEvents {
		list, _ := root[e.Event].([]any)
		kept := make([]any, 0, len(list))
		for _, entry := range list {
			if isCodexManagedEntry(entry) {
				continue
			}
			kept = append(kept, entry)
		}
		root[e.Event] = append(kept, managed[e.Event]...)
	}
	after, _ := json.Marshal(root)
	if string(before) == string(after) {
		return "unchanged", nil
	}
	if err := writeJSONObject(path, root); err != nil {
		return "", err
	}
	return "installed", nil
}

func removeCodexHooks(path string) (bool, error) {
	root, err := loadJSONObject(path)
	if err != nil {
		return false, err
	}
	changed := false
	for _, e := range codexHookEvents {
		list, ok := root[e.Event].([]any)
		if !ok {
			continue
		}
		kept := make([]any, 0, len(list))
		for _, entry := range list {
			if isCodexManagedEntry(entry) {
				changed = true
				continue
			}
			kept = append(kept, entry)
		}
		if len(kept) > 0 {
			root[e.Event] = kept
		} else {
			delete(root, e.Event)
		}
	}
	if !changed {
		return false, nil
	}
	if len(root) == 0 {
		// Nothing left — remove the file entirely.
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return false, err
		}
		return true, nil
	}
	if err := writeJSONObject(path, root); err != nil {
		return false, err
	}
	return true, nil
}

func codexHooksPresent(path string) bool {
	root, err := loadJSONObject(path)
	if err != nil {
		return false
	}
	for _, e := range codexHookEvents {
		list, _ := root[e.Event].([]any)
		found := false
		for _, entry := range list {
			if isCodexManagedEntry(entry) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func init() {
	recipes["codex"] = recipe{
		name: "codex", title: "Codex",
		install: codexInstall, check: codexCheck, remove: codexRemove,
	}
}
