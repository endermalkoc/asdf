package tutor

import (
	"regexp"
	"strings"

	"github.com/endermalkoc/cusp/internal/importer"
)

// This file splits a spec/entity Markdown body into its document sections and routes
// each to a curated section-type key (overview, purpose, …). Headings outside the
// curated vocabulary fold into the `notes` key (concatenated, each prefixed with its
// original `### heading` so nothing is lost). The FR/story parsers reconstruct the
// structural blocks; together a regenerate stays information-complete — conformed to
// the curated vocabulary (migration 0013).

var (
	h1Re          = regexp.MustCompile(`^#\s+(.+?)\s*$`)
	mandatoryRe   = regexp.MustCompile(`(?i)\s*[_*]?\(\s*(?:mandatory|optional)\s*\)[_*]?\s*$`)
	keyEntityRe   = regexp.MustCompile(`^\s*[-*]\s*\*\*([^*]+?)\*\*`)
	whyPriorityRe = regexp.MustCompile(`(?i)^\s*\*\*Why this priority\*\*:?\s*(.+)$`)
	indepTestRe   = regexp.MustCompile(`(?i)^\s*\*\*Independent Test\*\*:?\s*(.+)$`)

	// preambleFieldRe matches a `**Field**: value` metadata line in a spec preamble.
	preambleFieldRe = regexp.MustCompile(`(?i)^\s*\*\*([A-Za-z ]+?)\*\*\s*:\s*(.*)$`)
	isoDateRe       = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
)

// cleanSpecPreamble drops the boilerplate metadata lines from a spec preamble: Feature Branch
// (noise), Status (already a spec column, from frontmatter), and Updated (the row's
// updated_at). Created is captured into the returned date (→ spec.created_at) and likewise
// dropped. Every other line — notably `**Input**: User description: …` and any real prose —
// is kept. So the metadata lives in structured columns and is rendered from there, never
// duplicated as frozen prose.
func cleanSpecPreamble(preamble string) (cleaned, created string) {
	var kept []string
	for _, ln := range strings.Split(preamble, "\n") {
		if m := preambleFieldRe.FindStringSubmatch(ln); m != nil {
			switch strings.ToLower(strings.TrimSpace(m[1])) {
			case "feature branch", "status", "updated":
				continue // drop: noise or captured structurally
			case "created":
				created = isoDateRe.FindString(m[2])
				continue // captured into spec.created_at
			}
		}
		kept = append(kept, ln)
	}
	return strings.TrimSpace(strings.Join(kept, "\n")), created
}

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
	position int
	level    int
	heading  string
	body     string
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

	pos := 0
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
		pos++
		sections = append(sections, rawSection{position: pos, level: 2, heading: heading, body: strings.TrimSpace(strings.Join(b, "\n"))})
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

// specHeadingToKey / entityHeadingToKey map a normalized source heading to a curated
// section-type key. A heading absent from the map folds into `notes`. Every value here
// must be a seeded key (req_spec_section_type / ent_entity_section_type); apply skips and
// reports any key the seed does not know, so drift surfaces instead of corrupting data.
var specHeadingToKey = map[string]string{
	"overview":         "overview",
	"success criteria": "success_criteria",
	"platform scope":   "scope", // generalized from the corpus
	"assumptions":      "assumptions",
	"clarifications":   "open_questions", // generalized from the corpus
}

var entityHeadingToKey = map[string]string{
	"purpose":                "purpose",
	"key concepts":           "key_concepts",
	"schema reference":       "schema_reference",
	"relationships":          "relationships",
	"business rules":         "business_rules",
	"validations":            "validations",
	"row-level access rules": "access_control", // generalized from the corpus
	"notes":                  "notes",
	"spec references":        "references", // generalized from the corpus
}

// sectionAcc accumulates an owner's sections, merging any collisions on a curated key
// (notably `notes`, the fold target) so apply only ever sees one body per key — which is
// what UNIQUE(owner, section_type_key) demands. Key order is first-seen (source order);
// the generator re-orders by SectionType.position, so this order only affects the inner
// order of a merged body.
type sectionAcc struct {
	order []string
	parts map[string][]string
}

func newSectionAcc() *sectionAcc { return &sectionAcc{parts: map[string][]string{}} }

func (a *sectionAcc) add(key, body string) {
	body = strings.TrimSpace(body)
	if body == "" {
		return
	}
	if _, ok := a.parts[key]; !ok {
		a.order = append(a.order, key)
	}
	a.parts[key] = append(a.parts[key], body)
}

// fold appends content to the `notes` key, keeping the original heading as an inline
// `### heading` so the folded section stays legible.
func (a *sectionAcc) fold(heading, body string) {
	if h := strings.TrimSpace(heading); h != "" {
		if b := strings.TrimSpace(body); b != "" {
			a.add("notes", "### "+h+"\n\n"+b)
			return
		}
	}
	a.add("notes", body)
}

func (a *sectionAcc) sections() []importer.DocSection {
	out := make([]importer.DocSection, 0, len(a.order))
	for _, k := range a.order {
		out = append(out, importer.DocSection{Key: k, Body: strings.Join(a.parts[k], "\n\n")})
	}
	return out
}

// routeSpecSections captures a feature spec's prose into sp.Sections, each addressed by a
// curated key. Recognized headings map via specHeadingToKey; User Scenarios / Requirements
// are reconstructed elsewhere from the FR & story parsers (their Edge Cases / Key Entities
// subsections are extracted here), and every other heading folds into `notes`.
// (more_info is appended later by the FR parser — see Parse.)
func routeSpecSections(sp *importer.Spec, preamble string, sections []rawSection) {
	acc := newSectionAcc()
	cleaned, created := cleanSpecPreamble(preamble)
	sp.Created = created
	acc.add("preamble", cleaned)
	for _, s := range sections {
		switch norm := normHeading(s.heading); norm {
		case "key entities":
			sp.KeyEntities = parseKeyEntities(s.body) // → refs (queryable)
			acc.fold(s.heading, s.body)               // prose still round-trips, in notes
		case "user scenarios & testing", "user scenarios and testing":
			acc.add("edge_cases", subsection(s.body, "edge cases"))
			for _, ex := range sectionExtras(s.body, func(h string) bool {
				return strings.HasPrefix(h, "user story") || h == "edge cases"
			}) {
				acc.fold(ex.heading, ex.body)
			}
		case "requirements":
			if ke := subsection(s.body, "key entities"); ke != "" {
				sp.KeyEntities = parseKeyEntities(ke) // → refs (queryable)
			}
			for _, ex := range sectionExtras(s.body, func(h string) bool {
				return h == "functional requirements"
			}) {
				acc.fold(ex.heading, ex.body)
			}
		default:
			if key, ok := specHeadingToKey[norm]; ok {
				acc.add(key, s.body)
			} else {
				acc.fold(s.heading, s.body)
			}
		}
	}
	sp.Sections = acc.sections()
}

// extraSection is a fragment of a structural section that the row/FR parse does not cover.
type extraSection struct{ heading, body string }

// sectionExtras returns the prose a structural section's typed/row parse does not cover:
// the intro (before the first `###`, no heading) and each `###` subsection whose heading
// `skip` does not match.
func sectionExtras(body string, skip func(normalized string) bool) []extraSection {
	lines := strings.Split(body, "\n")
	var out []extraSection
	i := 0
	var intro []string
	for ; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "### ") {
			break
		}
		intro = append(intro, lines[i])
	}
	if s := strings.TrimSpace(strings.Join(intro, "\n")); s != "" {
		out = append(out, extraSection{body: s})
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
		out = append(out, extraSection{heading: h, body: strings.TrimSpace(strings.Join(bs, "\n"))})
	}
	return out
}

// routeEntitySections captures an entity doc's prose into e.Sections: the preamble, then
// recognized headings via entityHeadingToKey (purpose, key_concepts, …); every other
// heading folds into `notes` (merged with a genuine `## Notes`, if any).
func routeEntitySections(e *importer.Entity, preamble string, sections []rawSection) {
	acc := newSectionAcc()
	acc.add("preamble", preamble)
	for _, s := range sections {
		norm := normHeading(s.heading)
		if key, ok := entityHeadingToKey[norm]; ok {
			acc.add(key, s.body)
		} else {
			acc.fold(s.heading, s.body)
		}
	}
	e.Sections = acc.sections()
}
