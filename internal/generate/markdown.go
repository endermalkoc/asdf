package generate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/endermalkoc/cusp/internal/refs"
)

// The Markdown renderer: Model → Obsidian-flavored Markdown documents. It is pure (no
// store access) — it reads only the view structs Load assembled.

// markdownRenderer renders the Model as the Obsidian Markdown document tree. It holds the
// inline-ref resolver + priority labels so the per-document functions stay pure.
type markdownRenderer struct {
	res  *refs.Resolver
	prio map[int]string
}

func newMarkdownRenderer(m *Model) *markdownRenderer {
	return &markdownRenderer{res: refs.NewResolver(m.Targets), prio: m.Priorities}
}

// Render builds the full Markdown document tree: a page per spec/entity, the domain index
// + a landing page per domain, the entity index, and the glossary (when terms exist).
func (r *markdownRenderer) Render(m *Model) ([]File, error) {
	var files []File
	specPaths := map[string]bool{}
	for _, sp := range m.Specs {
		specPaths[sp.Path] = true
		files = append(files, File{Path: sp.Path, Content: markdownSpec(sp, r.res, r.prio), Kind: "spec"})
	}
	// Entities are first-class documents. Defensive: if a doc were ever both a spec and an
	// entity at the same path, the spec (already added) wins.
	for _, e := range m.Entities {
		if specPaths[e.DocPath] {
			continue
		}
		files = append(files, File{Path: e.DocPath, Content: markdownEntity(e, r.res), Kind: "entity"})
	}
	hasPlanning := len(m.Capabilities) > 0 || len(m.Deliverables) > 0 || len(m.Views) > 0
	files = append(files, File{Path: "index.md", Content: markdownDomainIndex(m.Domains, hasPlanning), Kind: "index"})
	specsByDomain := map[string][]*Spec{}
	for _, sp := range m.Specs {
		specsByDomain[sp.Domain] = append(specsByDomain[sp.Domain], sp)
	}
	for _, d := range m.Domains {
		files = append(files, File{Path: d.Slug + "/index.md", Content: markdownDomainPage(d, specsByDomain[d.Slug]), Kind: "index"})
	}
	files = append(files, File{Path: "entities/index.md", Content: markdownEntityIndex(m.Entities), Kind: "index"})
	if len(m.Terms) > 0 {
		files = append(files, File{Path: "glossary.md", Content: markdownGlossary(m.Terms, r.res), Kind: "glossary"})
	}
	if hasPlanning {
		files = append(files, File{Path: "planning/index.md", Content: markdownPlanningIndex(m), Kind: "planning"})
		if len(m.Capabilities) > 0 {
			files = append(files, File{Path: "planning/capabilities.md", Content: markdownCapabilities(m.Capabilities), Kind: "planning"})
		}
		if len(m.Deliverables) > 0 {
			files = append(files, File{Path: "planning/deliverables.md", Content: markdownDeliverables(m.Deliverables), Kind: "planning"})
		}
		if len(m.Views) > 0 {
			files = append(files, File{Path: "planning/views.md", Content: markdownViews(m.Views), Kind: "planning"})
		}
	}
	return files, nil
}

// ---- feature spec ----------------------------------------------------------

func markdownSpec(sp *Spec, res *refs.Resolver, prio map[int]string) string {
	var b strings.Builder
	rin := func(s string) string { out, _ := refs.RenderInline(s, sp.Path, res); return out }
	secs := newSections(sp.Sections)

	// Frontmatter carries the spec's structured metadata (the single source: the DB columns,
	// no longer the source preamble). HTML renders it as a styled bar; Obsidian shows it as
	// properties; JSON has the same fields. id appears only for a prefixed spec.
	b.WriteString("---\n")
	if sp.Prefix != "" {
		fmt.Fprintf(&b, "id: %s\n", sp.Prefix)
	}
	if sp.Title != "" {
		fmt.Fprintf(&b, "title: %s\n", sp.Title)
	}
	fmt.Fprintf(&b, "domain: %s\n", sp.Domain)
	fmt.Fprintf(&b, "status: %s\n", titleStatus(sp.Status))
	if sp.Created != "" {
		fmt.Fprintf(&b, "created: %s\n", sp.Created)
	}
	b.WriteString("---\n\n")

	fmt.Fprintf(&b, "# %s\n", heading(sp.Title, sp.Prefix))

	// Prose before the first structural anchor (preamble, overview).
	renderBand(&b, secs, rin, 0, posAnchorUserScenarios)

	// Anchor: User Scenarios & Testing — stories (+ rationale) → scenarios → Edge Cases.
	if len(sp.Stories) > 0 || secs.body("edge_cases") != "" {
		b.WriteString("\n## User Scenarios & Testing\n")
		for _, us := range sp.Stories {
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
			if len(us.Scenarios) > 0 {
				b.WriteString("\n**Acceptance Scenarios**:\n\n")
				for _, sc := range us.Scenarios {
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

	// Anchor: Requirements — FR list in document order, grouped by RequirementGroup.
	groupByID := map[string]*Group{}
	for _, g := range sp.Groups {
		groupByID[g.ID] = g
	}
	if len(sp.Requirements) > 0 {
		b.WriteString("\n## Requirements\n\n### Functional Requirements\n")
		curGroup := "\x00" // sentinel so the first row always emits its group context
		for _, r := range sp.Requirements {
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
			// An Obsidian block reference on every FR (end of the list item) so
			// [[REQ:fr-key]] resolves to it inside the spec (#^fr-key).
			anchor := strings.ToLower(r.FRKey)
			if r.Statement != "" {
				fmt.Fprintf(&b, "- **%s**: %s ^%s\n", r.FRKey, rin(r.Statement), anchor)
			} else {
				fmt.Fprintf(&b, "- **%s**: ^%s\n", r.FRKey, anchor)
			}
		}
	}
	// more_info: FR-area trailing prose, headingless, after the FR list.
	if mi := secs.body("more_info"); strings.TrimSpace(mi) != "" {
		fmt.Fprintf(&b, "\n%s\n", rin(strings.TrimRight(mi, "\n")))
	}

	// Prose after the last anchor (success_criteria, assumptions, scope, open_questions, notes).
	renderBand(&b, secs, rin, posAnchorRequirements, posEnd)
	return b.String()
}

// ---- entity doc ------------------------------------------------------------

func markdownEntity(e *Entity, res *refs.Resolver) string {
	var b strings.Builder
	rin := func(s string) string { out, _ := refs.RenderInline(s, e.DocPath, res); return out }
	secs := newSections(e.Sections)
	fmt.Fprintf(&b, "# %s\n", heading(e.Name))
	// preamble (headingless), then the Description→Purpose fallback (an entity column,
	// not a section), then the rest of the prose in canonical order.
	renderBand(&b, secs, rin, 0, posEntityPurpose)
	if secs.body("purpose") == "" && e.Description != "" {
		writeBlock(&b, "", rin(e.Description))
	}
	renderBand(&b, secs, rin, posEntityPurpose, posEnd)
	return b.String()
}

// ---- glossary --------------------------------------------------------------

func markdownGlossary(terms []*Term, res *refs.Resolver) string {
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

// ---- index pages -----------------------------------------------------------

func markdownDomainIndex(domains []*Domain, hasPlanning bool) string {
	sort.Slice(domains, func(i, j int) bool { return domains[i].Slug < domains[j].Slug })
	var b strings.Builder
	b.WriteString("# Feature Specifications\n\n")
	b.WriteString("Specifications organized by **domain**.\n\n")
	b.WriteString("## Domains\n\n| Domain | Description |\n|---|---|\n")
	for _, d := range domains {
		// Wikilink to the domain's index page; the alias pipe is escaped for the table cell.
		fmt.Fprintf(&b, "| [[%s/index\\|%s]] | %s |\n", d.Slug, domainLabel(d), d.Description)
	}
	seeAlso := "[[entities/index|Entities]]"
	if hasPlanning {
		seeAlso += ", [[planning/index|Planning]]"
	}
	fmt.Fprintf(&b, "\nSee also: %s.\n", seeAlso)
	return b.String()
}

// ---- planning roll-up pages ------------------------------------------------

// markdownPlanningIndex is the Planning landing: a short intro + a count table linking the
// capabilities / deliverables / views roll-ups.
func markdownPlanningIndex(m *Model) string {
	var b strings.Builder
	b.WriteString("# Planning\n\n")
	b.WriteString("What to build — **capabilities**, **deliverables**, and **views**, and how they relate.\n\n")
	b.WriteString("| Layer | Count |\n|---|---|\n")
	if len(m.Capabilities) > 0 {
		fmt.Fprintf(&b, "| [[planning/capabilities\\|Capabilities]] | %d |\n", len(m.Capabilities))
	}
	if len(m.Deliverables) > 0 {
		fmt.Fprintf(&b, "| [[planning/deliverables\\|Deliverables]] | %d |\n", len(m.Deliverables))
	}
	if len(m.Views) > 0 {
		fmt.Fprintf(&b, "| [[planning/views\\|Views]] | %d |\n", len(m.Views))
	}
	return b.String()
}

// markdownCapabilities renders capabilities grouped by domain as a nested list that mirrors
// the capability hierarchy (domain › epic › capability), so the structure is visible in
// Obsidian too. Each item carries its level, milestones, and deliverable count.
func markdownCapabilities(caps []*Capability) string {
	rootsByDomain, domains := buildCapForest(caps)
	var b strings.Builder
	b.WriteString("# Capabilities\n\n")
	b.WriteString("Product capabilities as a hierarchy — domain › epic › capability.\n")
	for _, d := range domains {
		fmt.Fprintf(&b, "\n## %s\n\n", humanizeSegment(d))
		for _, n := range rootsByDomain[d] {
			writeCapNodeMD(&b, n, 0)
		}
	}
	return b.String()
}

// writeCapNodeMD writes one capability (and its sub-capabilities) as an indented list item.
func writeCapNodeMD(b *strings.Builder, n *capTreeNode, depth int) {
	c := n.Cap
	line := strings.Repeat("  ", depth) + "- **" + mdInline(c.Title) + "**"
	if c.Level != "" {
		line += " `" + c.Level + "`"
	}
	var extra []string
	if len(c.Milestones) > 0 {
		extra = append(extra, strings.Join(c.Milestones, ", "))
	}
	if k := len(c.Deliverables); k > 0 {
		noun := "deliverables"
		if k == 1 {
			noun = "deliverable"
		}
		extra = append(extra, fmt.Sprintf("%d %s", k, noun))
	}
	if len(extra) > 0 {
		line += " — " + strings.Join(extra, " · ")
	}
	b.WriteString(line + "\n")
	for _, ch := range n.Children {
		writeCapNodeMD(b, ch, depth+1)
	}
}

// markdownDeliverables renders deliverables grouped by milestone, one table per milestone
// (Deliverable · Size · Status · AI-Ready · Capabilities · Views · Blocked by).
func markdownDeliverables(delivs []*Deliverable) string {
	byMilestone := map[string][]*Deliverable{}
	for _, d := range delivs {
		ms := d.Milestone
		if ms == "" {
			ms = "Unscheduled"
		}
		byMilestone[ms] = append(byMilestone[ms], d)
	}
	var b strings.Builder
	b.WriteString("# Deliverables\n\n")
	b.WriteString("Units of work, grouped by milestone. A title links to its source page when available.\n")
	for _, ms := range sortedKeys(byMilestone) {
		fmt.Fprintf(&b, "\n## %s\n\n", ms)
		b.WriteString("| Deliverable | Size | Status | AI-Ready | Capabilities | Views | Blocked by |\n|---|---|---|---|---|---|---|\n")
		for _, d := range byMilestone[ms] {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n",
				mdCell(d.Title), mdCell(d.Size), mdCell(titleStatus(d.Status)), mdCell(aiReadyLabel(d.AIReady)),
				mdCell(strings.Join(d.Capabilities, "; ")), mdCell(strings.Join(d.Views, "; ")), mdCell(strings.Join(d.BlockedBy, "; ")))
		}
	}
	return b.String()
}

// markdownViews renders views grouped by domain, one table per domain (View · Route · Spec ·
// Deliverables). The Spec cell wikilinks to the backing spec when one is resolved.
func markdownViews(views []*View) string {
	byDomain := map[string][]*View{}
	for _, v := range views {
		d := v.DomainSlug
		if d == "" {
			d = "unassigned"
		}
		byDomain[d] = append(byDomain[d], v)
	}
	var b strings.Builder
	b.WriteString("# Views\n\n")
	b.WriteString("UI surfaces, grouped by domain. The **Spec** column links the backing specification when one is set.\n")
	for _, d := range sortedKeys(byDomain) {
		fmt.Fprintf(&b, "\n## %s\n\n", humanizeSegment(d))
		b.WriteString("| View | Route | Spec | Deliverables |\n|---|---|---|---|\n")
		for _, v := range byDomain[d] {
			route := ""
			if v.Route != "" {
				route = "`" + v.Route + "`"
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n",
				mdCell(v.Title), route, viewSpecCell(v), mdCell(strings.Join(v.Deliverables, "; ")))
		}
	}
	return b.String()
}

// mdInline escapes the few Markdown metacharacters that would corrupt a title rendered in
// running text (a list item): emphasis/code markers and link brackets.
func mdInline(s string) string {
	r := strings.NewReplacer("*", "\\*", "_", "\\_", "`", "\\`", "[", "\\[", "]", "\\]")
	return r.Replace(strings.ReplaceAll(s, "\n", " "))
}

// viewSpecCell wikilinks to the backing spec (escaping the alias pipe for the table cell),
// or "—" when the view has no resolved spec.
func viewSpecCell(v *View) string {
	if v.SpecPath == "" {
		return "—"
	}
	label := v.SpecTitle
	if label == "" {
		label = strings.TrimSuffix(v.SpecPath, ".md")
	}
	return fmt.Sprintf("[[%s\\|%s]]", strings.TrimSuffix(v.SpecPath, ".md"), mdCell(label))
}

// aiReadyLabel humanizes the ai_ready enum for display (yes→Yes, no→No, na→N/A).
func aiReadyLabel(s string) string {
	switch s {
	case "yes":
		return "Yes"
	case "no":
		return "No"
	case "na":
		return "N/A"
	}
	return s
}

// mdCell makes a string safe inside a Markdown table cell: a literal pipe is escaped and
// newlines collapse to spaces.
func mdCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.ReplaceAll(s, "|", "\\|")
}

// sortedKeys returns a map's keys sorted, for deterministic grouped output.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// markdownDomainPage is a domain's landing page: its specs as wikilinks, in document order.
func markdownDomainPage(d *Domain, specs []*Spec) string {
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
	for _, sp := range specs { // model preserves ListSpecs order (path, slug)
		label := sp.Title
		if label == "" {
			label = sp.Slug
		}
		fmt.Fprintf(&b, "- [[%s|%s]]\n", strings.TrimSuffix(sp.Path, ".md"), label)
	}
	return b.String()
}

func markdownEntityIndex(entities []*Entity) string {
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

// ---- markdown helpers ------------------------------------------------------

func domainLabel(d *Domain) string {
	if d.Name != "" {
		return d.Name
	}
	return d.Slug
}

// roleWithArticle restores the indefinite article the importer strips from a user story's
// role ("tutor" → "a tutor"), so the narrative reads "As a tutor, …". A role that already
// carries an article is left as-is.
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

// titleStatus capitalizes a status to match the corpus convention (Draft/Active…).
func titleStatus(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// priorityText renders a priority level as "level - Label" (just the level if unknown).
func priorityText(prio map[int]string, level int) string {
	if l := prio[level]; l != "" {
		return fmt.Sprintf("%d - %s", level, l)
	}
	return fmt.Sprintf("%d", level)
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

// sections indexes an owner's prose sections by key (random access) and in render order.
type sections struct {
	byKey   map[string]*Section
	ordered []*Section
}

func newSections(rows []*Section) sections {
	s := sections{byKey: make(map[string]*Section, len(rows))}
	for _, r := range rows {
		s.byKey[r.Key] = r
		s.ordered = append(s.ordered, r) // Load preserves position order
	}
	return s
}

func (s sections) body(k string) string {
	if r := s.byKey[k]; r != nil {
		return r.Body
	}
	return ""
}

func (s sections) title(k string) string {
	if r := s.byKey[k]; r != nil {
		return r.Title
	}
	return ""
}

// level returns a section type's heading depth, defaulting to 2 (level-0 headingless rows
// are handled separately by renderBand/writeBlock).
func (s sections) level(k string) int {
	if r := s.byKey[k]; r != nil && r.Level > 0 {
		return r.Level
	}
	return 2
}

// Canonical-order band boundaries. renderBand renders prose sections by position; the two
// structural blocks are interleaved at these positions and own the block-owned types
// (edge_cases under User Scenarios, more_info under Requirements), which renderBand skips.
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
