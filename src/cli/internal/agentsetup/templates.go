// Package agentsetup installs Cusp's agent-integration artifacts into a repository — the
// instruction section injected into CLAUDE.md / AGENTS.md, the agent skill, and the SessionStart
// hook wiring — and keeps them idempotent via marker-delimited managed blocks. It holds no
// project- or host-specific state beyond these templates; the per-host installers live in the
// command layer (`cmd/cusp/setup*.go`).
package agentsetup

import _ "embed"

// SectionBody is the managed block injected into an agent instructions file (CLAUDE.md/AGENTS.md).
//
//go:embed templates/cusp-section.md
var SectionBody string

// SkillBody is the agent skill (SKILL.md) installed for skill-aware hosts.
//
//go:embed templates/SKILL.md
var SkillBody string

// OpenAISkillMeta is the OpenAI/Codex skill descriptor written alongside SKILL.md.
//
//go:embed templates/openai.yaml
var OpenAISkillMeta string
