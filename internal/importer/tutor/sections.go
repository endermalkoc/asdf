package tutor

import (
	"regexp"
	"strings"

	"github.com/endermalkoc/asdf/internal/importer"
)

// This file splits a spec/entity Markdown body into its document sections and
// routes them: recurring template sections → typed fields, everything else →
// the generic DocSection catch-all. Together with the FR/story parsers this makes
// a regenerate information-complete.

var (
	h1Re          = regexp.MustCompile(`^#\s+(.+?)\s*$`)
	mandatoryRe   = regexp.MustCompile(`(?i)\s*[_*]?\(\s*(?:mandatory|optional)\s*\)[_*]?\s*$`)
	keyEntityRe   = regexp.MustCompile(`^\s*[-*]\s*\*\*([^*]+?)\*\*`)
	whyPriorityRe = regexp.MustCompile(`(?i)^\s*\*\*Why this priority\*\*:?\s*(.+)$`)
	indepTestRe   = regexp.MustCompile(`(?i)^\s*\*\*Independent Test\*\*:?\s*(.+)$`)
)

// stripFrontmatter removes a leading `---`-delimited block from a body.
func stripFrontmatter(s string) string {
	lines := strings.Split(s, "\n")
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) || strings.TrimSpace(lines[i]) != "---" {
		return s
	}
	for j := i + 1; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == "---" {
			return strings.Join(lines[j+1:], "\n")
		}
	}
	return s
}

// normHeading lowercases a heading and strips a trailing _(mandatory)_/(optional) marker.
func normHeading(h string) string {
	return strings.ToLower(strings.TrimSpace(mandatoryRe.ReplaceAllString(h, "")))
}

type rawSection struct {
	ordinal int
	level   int
	heading string
	body    string
}

// splitDoc splits a body (after frontmatter) into the H1, the preamble (between
// the H1 and the first `##`), and the ordered list of top-level `##` sections
// (a section's body includes any `###` subsections).
func splitDoc(body string) (h1, preamble string, sections []rawSection) {
	lines := strings.Split(stripFrontmatter(body), "\n")
	i := 0
	for ; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			break
		}
		if m := h1Re.FindStringSubmatch(lines[i]); m != nil && !strings.HasPrefix(lines[i], "##") {
			h1 = strings.TrimSpace(m[1])
			i++
			break
		}
	}
	var pre []string
	for ; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			break
		}
		pre = append(pre, lines[i])
	}
	preamble = strings.TrimSpace(strings.Join(pre, "\n"))

	ord := 0
	for i < len(lines) {
		if !strings.HasPrefix(lines[i], "## ") {
			i++
			continue
		}
		heading := strings.TrimSpace(strings.TrimPrefix(lines[i], "## "))
		i++
		var b []string
		for i < len(lines) && !strings.HasPrefix(lines[i], "## ") {
			b = append(b, lines[i])
			i++
		}
		ord++
		sections = append(sections, rawSection{ordinal: ord, level: 2, heading: heading, body: strings.TrimSpace(strings.Join(b, "\n"))})
	}
	return h1, preamble, sections
}

// subsection returns the body of a `### <name>` subsection within a section body.
func subsection(body, name string) string {
	lines := strings.Split(body, "\n")
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "### ") {
			continue
		}
		if normHeading(strings.TrimPrefix(lines[i], "### ")) != strings.ToLower(name) {
			continue
		}
		var b []string
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(lines[j], "## ") || strings.HasPrefix(lines[j], "### ") {
				break
			}
			b = append(b, lines[j])
		}
		return strings.TrimSpace(strings.Join(b, "\n"))
	}
	return ""
}

// parseKeyEntities pulls the bold entity names out of a Key Entities bullet list.
func parseKeyEntities(body string) []string {
	var out []string
	seen := map[string]bool{}
	for _, ln := range strings.Split(body, "\n") {
		if m := keyEntityRe.FindStringSubmatch(ln); m != nil {
			name := strings.TrimSpace(m[1])
			if name != "" && !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	return out
}

// routeSpecSections captures a feature spec's sections into sp.Sections in source
// order: recognized template sections get a normalized section key (overview,
// edge_cases, …); User Scenarios / Requirements are reconstructed elsewhere from the
// FR & story parsers (their Edge Cases / Key Entities subsections are extracted here),
// and every other section is bespoke (empty key). Ordinals are source order; the
// generator renders keyed sections at canonical positions and bespoke ones by ordinal.
// (more_info is appended later by the FR parser — see Parse.)
func routeSpecSections(sp *importer.Spec, h1, preamble string, sections []rawSection) {
	sp.Heading = h1
	push := sectionPusher(&sp.Sections)
	if strings.TrimSpace(preamble) != "" {
		push(0, "", preamble, "preamble")
	}
	for _, s := range sections {
		switch normHeading(s.heading) {
		case "overview":
			push(s.level, s.heading, s.body, "overview")
		case "success criteria":
			push(s.level, s.heading, s.body, "success_criteria")
		case "platform scope":
			push(s.level, s.heading, s.body, "platform_scope")
		case "assumptions":
			push(s.level, s.heading, s.body, "assumptions")
		case "clarifications":
			push(s.level, s.heading, s.body, "clarifications")
		case "key entities":
			sp.KeyEntities = parseKeyEntities(s.body) // → refs (queryable)
			// Keep the section verbatim too (bespoke), so a top-level "## Key Entities"
			// round-trips its prose, not just the refs.
			push(s.level, s.heading, s.body, "")
		case "user scenarios & testing", "user scenarios and testing":
			if ec := subsection(s.body, "edge cases"); ec != "" {
				push(3, "Edge Cases", ec, "edge_cases")
			}
			// Preserve any intro prose / non-story, non-edge-case subsections (bespoke).
			for _, ex := range sectionExtras(s.body, func(h string) bool {
				return strings.HasPrefix(h, "user story") || h == "edge cases"
			}) {
				push(ex.Level, ex.Heading, ex.Body, "")
			}
		case "requirements":
			if ke := subsection(s.body, "key entities"); ke != "" {
				sp.KeyEntities = parseKeyEntities(ke) // → refs (queryable)
			}
			// Preserve intro prose + the Key Entities subsection verbatim (bespoke);
			// only the FR list is reconstructed from rows.
			for _, ex := range sectionExtras(s.body, func(h string) bool {
				return h == "functional requirements"
			}) {
				push(ex.Level, ex.Heading, ex.Body, "")
			}
		default:
			push(s.level, s.heading, s.body, "")
		}
	}
}

// sectionPusher returns a closure that appends a DocSection to *out with the next
// source-order ordinal. A recognized section passes its key; a bespoke one passes "".
func sectionPusher(out *[]importer.DocSection) func(level int, heading, body, key string) {
	return func(level int, heading, body, key string) {
		*out = append(*out, importer.DocSection{
			Ordinal: len(*out) + 1, Level: level, Heading: heading, Body: body, Key: key,
		})
	}
}

// sectionExtras returns the prose a structural section's typed/row parse does not
// cover: the intro (before the first `###`, emitted with no heading) and each
// `###` subsection whose heading `skip` does not match.
func sectionExtras(body string, skip func(normalized string) bool) []importer.DocSection {
	lines := strings.Split(body, "\n")
	var out []importer.DocSection
	i := 0
	var intro []string
	for ; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "### ") {
			break
		}
		intro = append(intro, lines[i])
	}
	if s := strings.TrimSpace(strings.Join(intro, "\n")); s != "" {
		out = append(out, importer.DocSection{Level: 0, Heading: "", Body: s})
	}
	for i < len(lines) {
		if !strings.HasPrefix(lines[i], "### ") {
			i++
			continue
		}
		h := strings.TrimSpace(strings.TrimPrefix(lines[i], "### "))
		i++
		var bs []string
		for i < len(lines) && !strings.HasPrefix(lines[i], "### ") && !strings.HasPrefix(lines[i], "## ") {
			bs = append(bs, lines[i])
			i++
		}
		if skip(normHeading(h)) {
			continue
		}
		out = append(out, importer.DocSection{Level: 3, Heading: h, Body: strings.TrimSpace(strings.Join(bs, "\n"))})
	}
	return out
}

// routeEntitySections captures an entity doc's sections into e.Sections in source
// order: the preamble (key "preamble") then recognized template sections with a
// normalized key (purpose, key_concepts, …); bespoke sections carry an empty key.
func routeEntitySections(e *importer.Entity, preamble string, sections []rawSection) {
	push := sectionPusher(&e.Sections)
	if strings.TrimSpace(preamble) != "" {
		push(0, "", preamble, "preamble")
	}
	for _, s := range sections {
		switch normHeading(s.heading) {
		case "purpose":
			push(s.level, s.heading, s.body, "purpose")
		case "key concepts":
			push(s.level, s.heading, s.body, "key_concepts")
		case "schema reference":
			push(s.level, s.heading, s.body, "schema_reference")
		case "relationships":
			push(s.level, s.heading, s.body, "relationships")
		case "business rules":
			push(s.level, s.heading, s.body, "business_rules")
		case "validations":
			push(s.level, s.heading, s.body, "validations")
		case "row-level access rules":
			push(s.level, s.heading, s.body, "row_level_access")
		case "notes":
			push(s.level, s.heading, s.body, "notes")
		case "spec references":
			push(s.level, s.heading, s.body, "spec_references")
		default:
			push(s.level, s.heading, s.body, "")
		}
	}
}
