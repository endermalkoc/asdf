// Package generate renders the canonical database back into Markdown — the
// git-ignored, read-only build artifacts ASDF's "generated, never edited"
// principle is built on. It reconstructs each Spec at its source-relative path
// (so the output tree mirrors the corpus and diffs cleanly against it), plus the
// domain and entity index pages.
//
// Goal: information-complete round-trip. Every document section the importer
// captured — typed template sections, promoted fields, the Clarifications log,
// Key Entities edges, and the generic DocSection tail — is re-emitted. The order
// is a fixed canonical one (sections may sit in a different place than the
// source); the information does not change.
package generate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/endermalkoc/asdf/internal/refs"
	"github.com/endermalkoc/asdf/internal/store"
)

// Stats tallies what was written.
type Stats struct {
	OutDir   string `json:"out_dir"`
	Specs    int    `json:"specs"`
	Entities int    `json:"entities"`
	Indexes  int    `json:"indexes"`
	Glossary int    `json:"glossary"`
}

// Total returns the file count.
func (s Stats) Total() int { return s.Specs + s.Entities + s.Indexes + s.Glossary }

// Generate reconstructs Markdown from x into outDir and returns what it wrote.
func Generate(ctx context.Context, x store.Execer, outDir string) (*Stats, error) {
	st := &Stats{OutDir: outDir}

	domains, err := store.ListDomains(ctx, x)
	if err != nil {
		return nil, err
	}
	specs, err := store.ListSpecs(ctx, x)
	if err != nil {
		return nil, err
	}
	entities, err := store.ListEntities(ctx, x)
	if err != nil {
		return nil, err
	}
	entityByPath := map[string]store.EntityRow{}
	for _, e := range entities {
		if e.DocPath != "" {
			entityByPath[e.DocPath] = e
		}
	}

	// Resolver for inline [[TYPE:key]] cross-references, built once over all targets.
	targets, err := store.ListRefTargets(ctx, x)
	if err != nil {
		return nil, err
	}
	res := refs.NewResolver(toTargets(targets))

	for _, sp := range specs {
		var md string
		if sp.Kind == "entity" {
			md, err = renderEntityDoc(ctx, x, sp, entityByPath[sp.Path], res)
			st.Entities++
		} else {
			md, err = renderSpec(ctx, x, sp, res)
			st.Specs++
		}
		if err != nil {
			return nil, err
		}
		if err := writeFile(outDir, sp.Path, md); err != nil {
			return nil, err
		}
	}

	if err := writeFile(outDir, "index.md", renderDomainIndex(domains)); err != nil {
		return nil, err
	}
	st.Indexes++
	if err := writeFile(outDir, "entities/index.md", renderEntityIndex(entities)); err != nil {
		return nil, err
	}
	st.Indexes++

	// Glossary page (when any terms are defined) — the [[TERM:slug]] link target.
	terms, err := store.ListGlossaryTerms(ctx, x)
	if err != nil {
		return nil, err
	}
	if len(terms) > 0 {
		if err := writeFile(outDir, "glossary.md", renderGlossary(terms, res)); err != nil {
			return nil, err
		}
		st.Glossary++
	}

	return st, nil
}

// renderGlossary builds the glossary page: one anchored section per term (so
// `[[TERM:slug]]` resolves to `glossary.md#slug`), with aliases and the definition
// (itself rendered, since a definition may contain inline cross-references).
func renderGlossary(terms []store.GlossaryTermRow, res *refs.Resolver) string {
	var b strings.Builder
	b.WriteString("# Glossary\n\n")
	b.WriteString("Shared project vocabulary. Reference a term inline with `[[TERM:slug]]`.\n")
	for _, t := range terms {
		name := t.Term
		if name == "" {
			name = t.Slug
		}
		fmt.Fprintf(&b, "\n## <a id=%q></a>%s\n", t.Slug, name)
		meta := "`[[TERM:" + t.Slug + "]]`"
		if len(t.Aliases) > 0 {
			meta += " · aka " + strings.Join(t.Aliases, ", ")
		}
		if t.DomainAbbrev != "" {
			meta += " · domain: " + t.DomainAbbrev
		}
		fmt.Fprintf(&b, "\n_%s_\n", meta)
		if strings.TrimSpace(t.Definition) != "" {
			def, _ := refs.RenderInline(t.Definition, "glossary.md", res)
			fmt.Fprintf(&b, "\n%s\n", strings.TrimRight(def, "\n"))
		}
	}
	return b.String()
}

// ---- feature spec ----------------------------------------------------------

func renderSpec(ctx context.Context, x store.Execer, sp store.SpecRow, res *refs.Resolver) (string, error) {
	var b strings.Builder
	rin := func(s string) string { out, _ := refs.RenderInline(s, sp.Path, res); return out }

	if sp.Prefix != "" {
		b.WriteString("---\n")
		fmt.Fprintf(&b, "id: %s\n", sp.Prefix)
		if sp.Title != "" {
			fmt.Fprintf(&b, "title: %s\n", sp.Title)
		}
		fmt.Fprintf(&b, "domain: %s\n", sp.DomainAbbrev)
		fmt.Fprintf(&b, "status: %s\n", titleStatus(sp.Status))
		b.WriteString("---\n\n")
	}

	fmt.Fprintf(&b, "# %s\n", heading(sp.Heading, sp.Title, sp.Prefix))
	writeBlock(&b, "", rin(sp.Preamble))
	writeSection(&b, 2, "Overview", rin(sp.Overview))

	// User Scenarios & Testing: stories (+ rationale) → scenarios → Edge Cases.
	stories, err := store.ListStoriesBySpec(ctx, x, sp.ID)
	if err != nil {
		return "", err
	}
	if len(stories) > 0 || sp.EdgeCases != "" {
		b.WriteString("\n## User Scenarios & Testing\n")
		for _, us := range stories {
			fmt.Fprintf(&b, "\n### User Story %d - %s (Priority: %s)\n", us.Ordinal, us.Title, us.Priority)
			if us.IWant != "" {
				narr := "As " + us.AsA + ", I want " + us.IWant
				if us.SoThat != "" {
					narr += " so that " + us.SoThat
				}
				fmt.Fprintf(&b, "\n%s.\n", rin(strings.TrimRight(narr, ".")))
			} else if us.Narrative != "" {
				fmt.Fprintf(&b, "\n%s\n", rin(strings.TrimRight(us.Narrative, "\n")))
			}
			if us.WhyPriority != "" {
				fmt.Fprintf(&b, "\n**Why this priority**: %s\n", rin(us.WhyPriority))
			}
			if us.IndependentTest != "" {
				fmt.Fprintf(&b, "\n**Independent Test**: %s\n", rin(us.IndependentTest))
			}
			scns, err := store.ListScenariosByStory(ctx, x, us.ID)
			if err != nil {
				return "", err
			}
			if len(scns) > 0 {
				b.WriteString("\n**Acceptance Scenarios**:\n\n")
				for _, sc := range scns {
					if sc.When != "" {
						fmt.Fprintf(&b, "%d. **Given** %s, **When** %s, **Then** %s\n", sc.Ordinal, rin(sc.Given), rin(sc.When), rin(sc.Then))
					} else {
						fmt.Fprintf(&b, "%d. **Given** %s, **Then** %s\n", sc.Ordinal, rin(sc.Given), rin(sc.Then))
					}
				}
			}
		}
		writeSection(&b, 3, "Edge Cases", rin(sp.EdgeCases))
	}

	// Requirements: FR list in document order, grouped by RequirementGroup (header
	// + interspersed note). The Requirements-section intro prose and the Key
	// Entities subsection round-trip via doc_sections (edges stay the queryable
	// form of Key Entities).
	reqs, err := store.ListReqsBySpecID(ctx, x, sp.ID)
	if err != nil {
		return "", err
	}
	groups, err := store.ListReqGroups(ctx, x, sp.ID)
	if err != nil {
		return "", err
	}
	groupByID := map[string]store.ReqGroupRow{}
	for _, g := range groups {
		groupByID[g.ID] = g
	}
	if len(reqs) > 0 {
		b.WriteString("\n## Requirements\n\n### Functional Requirements\n")
		curGroup := "\x00" // sentinel so the first row always emits its group context
		for _, r := range reqs {
			if r.GroupID != curGroup {
				curGroup = r.GroupID
				if g, ok := groupByID[r.GroupID]; ok {
					fmt.Fprintf(&b, "\n**%s**\n", g.Header)
					if strings.TrimSpace(g.Note) != "" {
						fmt.Fprintf(&b, "\n%s\n", rin(strings.TrimRight(g.Note, "\n")))
					}
					b.WriteString("\n")
				} else {
					b.WriteString("\n")
				}
			}
			// An anchor on every FR so [[REQ:fr-key]] links can target it inside the spec.
			anchor := strings.ToLower(r.FRKey)
			if r.Statement != "" {
				fmt.Fprintf(&b, "- <a id=%q></a>**%s**: %s\n", anchor, r.FRKey, rin(r.Statement))
			} else {
				fmt.Fprintf(&b, "- <a id=%q></a>**%s**:\n", anchor, r.FRKey)
			}
		}
	}
	// FR-area content that isn't an FR or a real group (note-only headers, config,
	// tables) — rendered verbatim at the end of the Requirements section.
	if strings.TrimSpace(sp.MoreInfo) != "" {
		fmt.Fprintf(&b, "\n%s\n", rin(strings.TrimRight(sp.MoreInfo, "\n")))
	}

	writeSection(&b, 2, "Success Criteria", rin(sp.SuccessCriteria))
	writeSection(&b, 2, "Platform Scope", rin(sp.PlatformScope))
	writeSection(&b, 2, "Assumptions", rin(sp.Assumptions))
	writeSection(&b, 2, "Clarifications", rin(sp.Clarifications))

	if err := writeDocSections(ctx, x, &b, "spec", sp.ID, sp.Path, res); err != nil {
		return "", err
	}
	return b.String(), nil
}

// ---- entity doc ------------------------------------------------------------

func renderEntityDoc(ctx context.Context, x store.Execer, sp store.SpecRow, e store.EntityRow, res *refs.Resolver) (string, error) {
	var b strings.Builder
	rin := func(s string) string { out, _ := refs.RenderInline(s, sp.Path, res); return out }
	fmt.Fprintf(&b, "# %s\n", heading(sp.Heading, e.Name, sp.Title))
	writeBlock(&b, "", rin(sp.Preamble))
	if e.Purpose == "" && e.Description != "" {
		writeBlock(&b, "", rin(e.Description))
	}
	writeSection(&b, 2, "Purpose", rin(e.Purpose))
	writeSection(&b, 2, "Key Concepts", rin(e.KeyConcepts))
	writeSection(&b, 2, "Schema Reference", rin(e.SchemaReference))
	writeSection(&b, 2, "Relationships", rin(e.Relationships))
	writeSection(&b, 2, "Business Rules", rin(e.BusinessRules))
	writeSection(&b, 2, "Validations", rin(e.Validations))
	writeSection(&b, 2, "Row-Level Access Rules", rin(e.RowLevelAccess))
	writeSection(&b, 2, "Notes", rin(e.Notes))
	writeSection(&b, 2, "Spec References", rin(e.SpecReferences))
	if e.ID != "" {
		if err := writeDocSections(ctx, x, &b, "entity", e.ID, sp.Path, res); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

// ---- index pages -----------------------------------------------------------

func renderDomainIndex(domains []store.Domain) string {
	sort.Slice(domains, func(i, j int) bool { return domains[i].Abbreviation < domains[j].Abbreviation })
	var b strings.Builder
	b.WriteString("# Feature Specifications\n\n")
	b.WriteString("Specifications organized by **domain**.\n\n")
	b.WriteString("## Domains\n\n| Domain | Description |\n|---|---|\n")
	for _, d := range domains {
		fmt.Fprintf(&b, "| [%s/](./%s/) | %s |\n", d.Abbreviation, d.Abbreviation, d.Description)
	}
	return b.String()
}

func renderEntityIndex(entities []store.EntityRow) string {
	sort.Slice(entities, func(i, j int) bool { return entities[i].Name < entities[j].Name })
	var b strings.Builder
	b.WriteString("# Entities\n\n")
	b.WriteString("Shared entity definitions.\n\n")
	b.WriteString("## Entities\n\n| Entity | Description | Status |\n|---|---|---|\n")
	for _, e := range entities {
		link := e.Name
		if e.DocPath != "" {
			link = fmt.Sprintf("[%s](./%s)", e.Name, filepath.Base(e.DocPath))
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", link, e.Description, titleStatus(e.Status))
	}
	return b.String()
}

// ---- helpers ---------------------------------------------------------------

// heading picks the first non-empty of the candidates.
func heading(cands ...string) string {
	for _, c := range cands {
		if strings.TrimSpace(c) != "" {
			return c
		}
	}
	return ""
}

// writeSection emits "<level heading>\n\n<body>\n" when body is non-empty.
func writeSection(b *strings.Builder, level int, head, body string) {
	if strings.TrimSpace(body) == "" {
		return
	}
	fmt.Fprintf(b, "\n%s %s\n\n%s\n", strings.Repeat("#", level), head, strings.TrimRight(body, "\n"))
}

// writeBlock emits a bare body block (no heading) when non-empty.
func writeBlock(b *strings.Builder, _ string, body string) {
	if strings.TrimSpace(body) == "" {
		return
	}
	fmt.Fprintf(b, "\n%s\n", strings.TrimRight(body, "\n"))
}

// writeDocSections re-emits an owner's generic catch-all sections in order, with
// inline cross-references rendered.
func writeDocSections(ctx context.Context, x store.Execer, b *strings.Builder, ownerType, ownerID, ownerDocPath string, res *refs.Resolver) error {
	secs, err := store.ListDocSections(ctx, x, ownerType, ownerID)
	if err != nil {
		return err
	}
	for _, s := range secs {
		body, _ := refs.RenderInline(s.Body, ownerDocPath, res)
		if s.Level == 0 || strings.TrimSpace(s.Heading) == "" {
			writeBlock(b, "", body) // intro prose, no heading
			continue
		}
		level := s.Level
		if level < 2 {
			level = 2
		}
		writeSection(b, level, s.Heading, body)
	}
	return nil
}

// toTargets adapts store ref-target rows to the refs resolver's Target shape.
func toTargets(rows []store.RefTargetRow) []refs.Target {
	out := make([]refs.Target, len(rows))
	for i, r := range rows {
		out[i] = refs.Target{Type: r.Type, Key: r.Key, ID: r.ID, DocPath: r.DocPath, Anchor: r.Anchor}
	}
	return out
}

// titleStatus capitalizes a status to match the corpus convention (Draft/Active…).
func titleStatus(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// writeFile writes content to outDir/relPath, creating parent directories.
func writeFile(outDir, relPath, content string) error {
	target := filepath.Join(outDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(target, []byte(content), 0o644)
}
