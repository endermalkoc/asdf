// Package tutor is the source adapter for the "tutor" documentation corpus
// (markdown feature specs + a parallel fr-registry/ of YAML coverage sidecars).
// It is a deterministic, read-only parser: it walks the corpus and produces an
// importer.Graph (Domain/Spec/Requirement/UserStory/Scenario/Edge/Milestone)
// plus an importer.Report of drift and ER gaps. No database, no LLM.
//
// Authority model (what each corpus part is the source of truth for):
//   - specs/index.md           → the domain set (name + description)
//   - specs/prefix-registry.md → the spec set (prefix → path → domain → status)
//   - fr-registry/**.yml       → the requirement set (fr_key → delivery status,
//     milestone, notes) — authoritative existence + coverage
//   - specs/**/*.md frontmatter → spec title + source status
//   - specs/**/*.md FR lines    → requirement statement text
//   - specs/**/*.md stories      → user stories + acceptance scenarios
//
// Requirements are anchored on the registry (clean YAML) and enriched with
// statements from the bold FR lines; every mismatch becomes a Report finding.
package tutor

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/endermalkoc/cusp/internal/importer"
)

// Parse reads the tutor corpus rooted at docsRoot (the directory that contains
// specs/ and fr-registry/) and returns the staging graph and a parse report.
// resolveDrizzleDir returns the Drizzle schema directory: the explicit override (if it
// exists), else the conventional tutor location relative to the docs root, else "".
func resolveDrizzleDir(docsRoot, override string) string {
	if override != "" {
		if fi, err := os.Stat(override); err == nil && fi.IsDir() {
			return override
		}
		return ""
	}
	auto := filepath.Join(docsRoot, "..", "src", "packages", "database", "src", "schema")
	if fi, err := os.Stat(auto); err == nil && fi.IsDir() {
		return auto
	}
	return ""
}

// Parse builds the staging graph from the tutor docs corpus. drizzleDir is an optional
// override for the Drizzle schema (entity relationships); "" auto-detects the tutor
// location relative to docsRoot.
func Parse(docsRoot, drizzleDir string) (*importer.Graph, *importer.Report, error) {
	specsDir := filepath.Join(docsRoot, "specs")
	registryDir := filepath.Join(docsRoot, "fr-registry")
	if _, err := os.Stat(specsDir); err != nil {
		return nil, nil, fmt.Errorf("specs/ not found under %s: %w", docsRoot, err)
	}

	g := &importer.Graph{}
	rep := &importer.Report{Counts: map[string]int{}, Coverage: map[string]int{}}

	// 1. Domains (specs/index.md).
	domains, err := parseDomains(filepath.Join(specsDir, "index.md"))
	if err != nil {
		return nil, nil, err
	}
	g.Domains = domains
	domainSet := map[string]bool{}
	for _, d := range domains {
		domainSet[d.Slug] = true
	}

	// 2. Specs (prefix-registry.md), enriched from frontmatter.
	specs, fmStatus, err := parseSpecs(specsDir, rep, domainSet)
	if err != nil {
		return nil, nil, err
	}
	g.Specs = specs
	_ = fmStatus

	// 3. Per-spec markdown: document sections + FR statements + user stories + scenarios.
	stmtByKey := map[string]frStatement{} // fr_key → statement parsed from a spec line
	roleUnparsed := 0
	for i := range g.Specs {
		sp := &g.Specs[i]
		if sp.Prefix == "" {
			continue
		}
		body, ok := readSpecBody(specsDir, sp.Path)
		if !ok {
			continue // missing-file finding already recorded in parseSpecs
		}
		_, preamble, secs := splitDoc(body)
		routeSpecSections(sp, preamble, secs)
		var moreInfo string
		sp.ReqGroups, moreInfo = collectStatements(sp.Prefix, body, stmtByKey, rep)
		// FR-area trailing prose (note-only headers, config, tables) → the `more_info`
		// section, rendered after the FR list (the generator looks it up by key).
		if strings.TrimSpace(moreInfo) != "" {
			sp.Sections = append(sp.Sections, importer.DocSection{Key: "more_info", Body: moreInfo})
		}
		stories, scenarios, ru := parseStories(sp.Prefix, body)
		roleUnparsed += ru
		g.Stories = append(g.Stories, stories...)
		g.Scenarios = append(g.Scenarios, scenarios...)
	}

	// 3b. Entity glossary (entities/index.md) → first-class Entity rows (not specs).
	g.Entities = parseEntities(specsDir, rep)
	g.Specs = dedupeSpecsByPath(g.Specs) // defensive: registry spec paths should be unique

	// 3c. Entity relationships from the Drizzle schema (optional source).
	if dir := resolveDrizzleDir(docsRoot, drizzleDir); dir != "" {
		names := make([]string, len(g.Entities))
		for i, e := range g.Entities {
			names[i] = e.Name
		}
		g.Relationships = parseDrizzle(dir, names, rep)
		rep.Counts["entity_relationships"] = len(g.Relationships)
	} else if drizzleDir != "" {
		rep.Add(importer.SevWarn, "drizzle-not-found", "--drizzle schema dir not found: "+drizzleDir, "")
	}

	// 4. Requirements (fr-registry/**.yml), joined to statements.
	reqs, milestones, err := parseRegistry(registryDir, stmtByKey, rep)
	if err != nil {
		return nil, nil, err
	}
	g.Reqs = reqs
	g.Milestones = milestones

	// 5. Cross-references → entity_ref: inline FR mentions, Key Entities, and
	// converted `[label](./other.md)` cross-spec/entity links (rewritten in place
	// to canonical [[TYPE:key]] tokens).
	g.Refs = convertRefs(g, rep)

	// 6. Cross-checks and counts.
	buildReport(g, stmtByKey, rep)
	crossCheckCoverage(docsRoot, g, rep)

	// Story-narrative classification: genuine parse misses vs prose-style stories
	// (which carry no "As a … I want … so that …" line by authoring choice).
	if roleUnparsed > 0 {
		rep.Add(importer.SevWarn, "story-narrative-unparsed",
			itoa(roleUnparsed)+" role-style stories have an 'I want' line the parser could not split (multi-line/odd phrasing)", "")
	}
	proseStyle := 0
	for _, s := range g.Stories {
		if s.IWant == "" {
			proseStyle++
		}
	}
	proseStyle -= roleUnparsed
	if proseStyle > 0 {
		rep.Add(importer.SevInfo, "story-prose-style",
			itoa(proseStyle)+" user stories are prose-style (no 'As a … I want …' narrative) — heading + scenarios captured, role fields empty", "")
	}

	return g, rep, nil
}

// ---- shared patterns -------------------------------------------------------

var (
	// frTokenRe matches an FR id anywhere in text: PREFIX-FR-NNN with optional sub-letter.
	frTokenRe = regexp.MustCompile(`\b([A-Z]{2,6})-FR-(\d{2,3})([a-z]?)\b`)

	// frLineRe matches a bold FR definition list item, capturing an optional
	// [visual]/[operational]/[untestable] marker and the statement text.
	//   - **ADDS-FR-001**: System MUST require first name and last name
	//   - **ATT-FR-030**: `[visual]` Red dot on unsaved rows
	frLineRe = regexp.MustCompile(
		"^\\s*[-*]\\s+\\*\\*([A-Z]{2,6})-FR-(\\d{2,3})([a-z]?)\\*\\*:\\s*(?:`?\\[(visual|operational|untestable)\\]`?\\s*)?(.*)$")

	// frRangeLineRe matches a delegated range list item:
	//   - **ADDS-FR-004 to ADDS-FR-005**: Email and phone validation follow [...]
	frRangeLineRe = regexp.MustCompile(
		`^\s*[-*]\s+\*\*([A-Z]{2,6})-FR-(\d{2,3})([a-z]?)\s+to\s+([A-Z]{2,6})-FR-(\d{2,3})([a-z]?)\*\*:\s*(.*)$`)
)

// frKey formats a citation key from its parts.
func frKey(prefix string, number int, suffix string) string {
	return fmt.Sprintf("%s-FR-%03d%s", prefix, number, suffix)
}

// splitFRKey parses "ADDS-FR-012a" into (ADDS, 12, "a"). ok=false if malformed.
func splitFRKey(key string) (prefix string, number int, suffix string, ok bool) {
	m := frTokenRe.FindStringSubmatch(key)
	if m == nil || m[0] != key {
		return "", 0, "", false
	}
	n := 0
	for _, c := range m[2] {
		n = n*10 + int(c-'0')
	}
	return m[1], n, m[3], true
}

// dedupeSpecsByPath keeps the first spec for each path (the FR-bearing registry
// entry wins over a bare entity-doc spec at the same path).
func dedupeSpecsByPath(specs []importer.Spec) []importer.Spec {
	seen := map[string]bool{}
	var out []importer.Spec
	for _, s := range specs {
		if seen[s.Path] {
			continue
		}
		seen[s.Path] = true
		out = append(out, s)
	}
	return out
}

// readSpecBody reads a spec file by its registry-relative path.
func readSpecBody(specsDir, relPath string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(specsDir, relPath))
	if err != nil {
		return "", false
	}
	return string(b), true
}

// trimMarkdown strips trailing whitespace and a trailing period from a fragment.
func trimMarkdown(s string) string {
	return strings.TrimSpace(s)
}
