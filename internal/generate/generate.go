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

	// Resolver for inline [[TYPE:key]] cross-references, built once over all targets.
	targets, err := store.ListRefTargets(ctx, x)
	if err != nil {
		return nil, err
	}
	res := refs.NewResolver(toTargets(targets))

	prio, err := loadPriorityLabels(ctx, x)
	if err != nil {
		return nil, err
	}

	specPaths := map[string]bool{}
	for _, sp := range specs {
		md, err := renderSpec(ctx, x, sp, res, prio)
		if err != nil {
			return nil, err
		}
		st.Specs++
		path := specFullPath(sp)
		specPaths[path] = true
		if err := writeFile(outDir, path, md); err != nil {
			return nil, err
		}
	}

	// Entities are first-class documents (not specs): rendered from ent_entity +
	// ent_entity_section, to their own doc path. Defensive: if some doc were ever both a
	// spec and an entity at the same path, the spec (already written) wins — skip it here.
	for _, e := range entities {
		if specPaths[e.DocPath] {
			continue
		}
		md, err := renderEntityDoc(ctx, x, e, res)
		if err != nil {
			return nil, err
		}
		st.Entities++
		if err := writeFile(outDir, e.DocPath, md); err != nil {
			return nil, err
		}
	}

	if err := writeFile(outDir, "index.md", renderDomainIndex(domains)); err != nil {
		return nil, err
	}
	st.Indexes++
	// One index page per domain (so the root index's domain links resolve to a real
	// note, and each domain has a browsable landing page listing its specs).
	specsByDomain := map[string][]store.SpecRow{}
	for _, sp := range specs {
		specsByDomain[sp.DomainSlug] = append(specsByDomain[sp.DomainSlug], sp)
	}
	for _, d := range domains {
		if err := writeFile(outDir, d.Slug+"/index.md", renderDomainPage(d, specsByDomain[d.Slug])); err != nil {
			return nil, err
		}
		st.Indexes++
	}
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
		fmt.Fprintf(&b, "\n## %s\n", name)
		meta := "`[[TERM:" + t.Slug + "]]`"
		if len(t.Aliases) > 0 {
			meta += " · aka " + strings.Join(t.Aliases, ", ")
		}
		if t.DomainSlug != "" {
			meta += " · domain: " + t.DomainSlug
		}
		// Obsidian block reference so [[TERM:slug]] resolves to glossary.md#^slug.
		fmt.Fprintf(&b, "\n_%s_ ^%s\n", meta, t.Slug)
		if strings.TrimSpace(t.Definition) != "" {
			def, _ := refs.RenderInline(t.Definition, "glossary.md", res)
			fmt.Fprintf(&b, "\n%s\n", strings.TrimRight(def, "\n"))
		}
	}
	return b.String()
}

// ---- feature spec ----------------------------------------------------------

func renderSpec(ctx context.Context, x store.Execer, sp store.SpecRow, res *refs.Resolver, prio map[int]string) (string, error) {
	var b strings.Builder
	rin := func(s string) string { out, _ := refs.RenderInline(s, specFullPath(sp), res); return out }
	secs, err := loadSpecSections(ctx, x, sp.ID)
	if err != nil {
		return "", err
	}

	if sp.Prefix != "" {
		b.WriteString("---\n")
		fmt.Fprintf(&b, "id: %s\n", sp.Prefix)
		if sp.Title != "" {
			fmt.Fprintf(&b, "title: %s\n", sp.Title)
		}
		fmt.Fprintf(&b, "domain: %s\n", sp.DomainSlug)
		fmt.Fprintf(&b, "status: %s\n", titleStatus(sp.Status))
		b.WriteString("---\n\n")
	}

	fmt.Fprintf(&b, "# %s\n", heading(sp.Title, sp.Prefix))

	// Prose before the first structural anchor (preamble, overview).
	renderBand(&b, secs, rin, 0, posAnchorUserScenarios)

	// Anchor: User Scenarios & Testing — stories (+ rationale) → scenarios → Edge Cases.
	stories, err := store.ListStoriesBySpec(ctx, x, sp.ID)
	if err != nil {
		return "", err
	}
	if len(stories) > 0 || secs.body("edge_cases") != "" {
		b.WriteString("\n## User Scenarios & Testing\n")
		for _, us := range stories {
			fmt.Fprintf(&b, "\n### User Story %d - %s (Priority: %s)\n", us.Position, us.Title, priorityText(prio, us.Priority))
			if us.IWant != "" {
				narr := "As " + roleWithArticle(us.AsA) + ", I want " + us.IWant
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
						fmt.Fprintf(&b, "%d. **Given** %s, **When** %s, **Then** %s\n", sc.Position, rin(sc.Given), rin(sc.When), rin(sc.Then))
					} else {
						fmt.Fprintf(&b, "%d. **Given** %s, **Then** %s\n", sc.Position, rin(sc.Given), rin(sc.Then))
					}
				}
			}
		}
		writeSection(&b, secs.level("edge_cases"), secs.title("edge_cases"), rin(secs.body("edge_cases")))
	}

	// Prose between the anchors (none today; future-proof).
	renderBand(&b, secs, rin, posAnchorUserScenarios, posAnchorRequirements)

	// Anchor: Requirements — FR list in document order, grouped by RequirementGroup
	// (header + interspersed note). The Requirements intro prose and the Key Entities
	// subsection fold into `notes`; edges stay the queryable form of Key Entities.
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
					fmt.Fprintf(&b, "\n**%s**\n", g.Title)
					if strings.TrimSpace(g.Notes) != "" {
						fmt.Fprintf(&b, "\n%s\n", rin(strings.TrimRight(g.Notes, "\n")))
					}
					b.WriteString("\n")
				} else {
					b.WriteString("\n")
				}
			}
			// An Obsidian block reference on every FR (at the end of the list item) so
			// [[REQ:fr-key]] links resolve to it inside the spec (#^fr-key).
			anchor := strings.ToLower(r.FRKey)
			if r.Statement != "" {
				fmt.Fprintf(&b, "- **%s**: %s ^%s\n", r.FRKey, rin(r.Statement), anchor)
			} else {
				fmt.Fprintf(&b, "- **%s**: ^%s\n", r.FRKey, anchor)
			}
		}
	}
	// more_info: FR-area trailing prose, headingless, after the FR list (emitted even
	// when there are no FRs, matching the source).
	if mi := secs.body("more_info"); strings.TrimSpace(mi) != "" {
		fmt.Fprintf(&b, "\n%s\n", rin(strings.TrimRight(mi, "\n")))
	}

	// Prose after the last anchor (success_criteria, assumptions, scope, open_questions, notes).
	renderBand(&b, secs, rin, posAnchorRequirements, posEnd)
	return b.String(), nil
}

// ---- entity doc ------------------------------------------------------------

func renderEntityDoc(ctx context.Context, x store.Execer, e store.EntityRow, res *refs.Resolver) (string, error) {
	var b strings.Builder
	rin := func(s string) string { out, _ := refs.RenderInline(s, e.DocPath, res); return out }
	secs, err := loadEntitySections(ctx, x, e.ID)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&b, "# %s\n", heading(e.Name))
	// preamble (headingless), then the Description→Purpose fallback (an entity column,
	// not a section), then the rest of the prose in canonical order.
	renderBand(&b, secs, rin, 0, posEntityPurpose)
	if secs.body("purpose") == "" && e.Description != "" {
		writeBlock(&b, "", rin(e.Description))
	}
	renderBand(&b, secs, rin, posEntityPurpose, posEnd)
	return b.String(), nil
}

// ---- index pages -----------------------------------------------------------

func renderDomainIndex(domains []store.Domain) string {
	sort.Slice(domains, func(i, j int) bool { return domains[i].Slug < domains[j].Slug })
	var b strings.Builder
	b.WriteString("# Feature Specifications\n\n")
	b.WriteString("Specifications organized by **domain**.\n\n")
	b.WriteString("## Domains\n\n| Domain | Description |\n|---|---|\n")
	for _, d := range domains {
		// Wikilink to the domain's index page; the alias pipe is escaped for the table cell.
		fmt.Fprintf(&b, "| [[%s/index\\|%s]] | %s |\n", d.Slug, domainLabel(d), d.Description)
	}
	b.WriteString("\nSee also: [[entities/index|Entities]].\n")
	return b.String()
}

func domainLabel(d store.Domain) string {
	if d.Name != "" {
		return d.Name
	}
	return d.Slug
}

// renderDomainPage is a domain's landing page: its specs as wikilinks, in document order.
func renderDomainPage(d store.Domain, specs []store.SpecRow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", domainLabel(d))
	if d.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", d.Description)
	}
	b.WriteString("## Specs\n\n")
	if len(specs) == 0 {
		b.WriteString("_No specs yet._\n")
		return b.String()
	}
	for _, sp := range specs { // ListSpecs orders by path, slug
		label := sp.Title
		if label == "" {
			label = sp.Slug
		}
		fmt.Fprintf(&b, "- [[%s|%s]]\n", strings.TrimSuffix(specFullPath(sp), ".md"), label)
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
			// Wikilink; the alias pipe is escaped for the table cell.
			link = fmt.Sprintf("[[%s\\|%s]]", strings.TrimSuffix(e.DocPath, ".md"), e.Name)
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", link, e.Description, titleStatus(e.Status))
	}
	return b.String()
}

// ---- helpers ---------------------------------------------------------------

// roleWithArticle restores the indefinite article the importer strips from a user
// story's role ("tutor" → "a tutor"), so the rendered narrative reads "As a tutor, …".
// A role that already carries an article ("an …", "the …") is left as-is — the importer
// only strips a leading "a ", so those survive in the stored value.
func roleWithArticle(role string) string {
	if role == "" {
		return role
	}
	lower := strings.ToLower(role)
	if strings.HasPrefix(lower, "a ") || strings.HasPrefix(lower, "an ") || strings.HasPrefix(lower, "the ") {
		return role
	}
	return "a " + role
}

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

// sections is an owner's prose sections, each joined to its curated type — indexed both
// by key (random access to body/title/level) and in canonical render order.
type sections struct {
	byKey   map[string]store.SectionRow
	ordered []store.SectionRow
}

func newSections(rows []store.SectionRow) sections {
	s := sections{byKey: make(map[string]store.SectionRow, len(rows))}
	for _, r := range rows {
		s.byKey[r.Key] = r
		s.ordered = append(s.ordered, r) // store.List* already orders by position
	}
	return s
}

func (s sections) body(k string) string  { return s.byKey[k].Body }
func (s sections) title(k string) string { return s.byKey[k].Title }

// level returns a section type's heading depth, defaulting to 2 (level-0 headingless
// rows are handled separately by renderBand/writeBlock).
func (s sections) level(k string) int {
	if l := s.byKey[k].Level; l > 0 {
		return l
	}
	return 2
}

func loadSpecSections(ctx context.Context, x store.Execer, specID string) (sections, error) {
	rows, err := store.ListSpecSections(ctx, x, specID)
	if err != nil {
		return sections{}, err
	}
	return newSections(rows), nil
}

func loadEntitySections(ctx context.Context, x store.Execer, entityID string) (sections, error) {
	rows, err := store.ListEntitySections(ctx, x, entityID)
	if err != nil {
		return sections{}, err
	}
	return newSections(rows), nil
}

// Canonical-order band boundaries. renderBand renders prose sections by
// SectionType.position; the two structural blocks are interleaved at these positions and
// own the block-owned types (edge_cases under User Scenarios, more_info under
// Requirements), which renderBand skips. The seed's positions must straddle these.
const (
	posAnchorUserScenarios = 25
	posAnchorRequirements  = 35
	posEntityPurpose       = 10 // the entity Description→Purpose fallback slots just before this
	posEnd                 = 1 << 30
)

// blockOwned section types render inside their structural anchor, not the generic sweep.
var blockOwned = map[string]bool{"edge_cases": true, "more_info": true}

// renderBand emits prose sections whose position is in [lo,hi), in order, skipping
// block-owned types; a level-0 (headingless) type renders as a bare block.
func renderBand(b *strings.Builder, secs sections, rin func(string) string, lo, hi int) {
	for _, s := range secs.ordered {
		if s.Position < lo || s.Position >= hi || blockOwned[s.Key] {
			continue
		}
		if s.Level == 0 {
			writeBlock(b, "", rin(s.Body))
		} else {
			writeSection(b, s.Level, s.Title, rin(s.Body))
		}
	}
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
// loadPriorityLabels loads the level→label map for the 0–4 priority taxonomy.
func loadPriorityLabels(ctx context.Context, x store.Execer) (map[int]string, error) {
	rows, err := store.ListPriorities(ctx, x)
	if err != nil {
		return nil, err
	}
	m := make(map[int]string, len(rows))
	for _, p := range rows {
		m[p.Level] = p.Label
	}
	return m, nil
}

// priorityText renders a priority level as "level - Label" (just the level if unknown).
func priorityText(prio map[int]string, level int) string {
	if l := prio[level]; l != "" {
		return fmt.Sprintf("%d - %s", level, l)
	}
	return fmt.Sprintf("%d", level)
}

// specFullPath reconstructs a spec's full docs path: <domain>/[path/]<slug>.md
// (req_spec.path is the directory only; the filename is slug.md — migration 0017).
func specFullPath(sp store.SpecRow) string {
	return store.SpecDocPath(sp.DomainSlug, sp.Path, sp.Slug)
}

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
