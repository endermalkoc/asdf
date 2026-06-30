package tutor

import (
	"regexp"
	"strings"

	"github.com/endermalkoc/cusp/internal/importer"
)

var (
	// Priority appears as "(Priority: P1)", "(Priority: P1, Milestone: M4)", or a
	// bare "(P1)"; the "Priority:" prefix and trailing fields are optional, and the
	// level may exceed P3 in a few specs.
	storyHeadingRe = regexp.MustCompile(`^###\s+User Story\s+(\d+)\s*[-\x{2013}\x{2014}]\s*(.+?)\s*\((?:Priority:\s*)?(P[1-9])[^)]*\)\s*$`)

	// asA matches the story narrative: "As <role>, I want X so that Y." The capture
	// keeps the article ("a tutor", "the studio"); a leading "a " is stripped at the
	// assignment below ("a tutor" → "tutor"). The connector is "so that" or a bare
	// "so" (… so I can …, … so they …).
	asARe      = regexp.MustCompile(`(?i)\bas (\w.+?),?\s+i want\s+(.+?)\s+so (?:that )?(.+?)\.?\s*$`)
	asAStartRe = regexp.MustCompile(`(?i)^\s*as\s+\w`)

	// scenarioRe matches "1. **Given** … **When** … **Then** …"; the When clause is
	// optional (some scenarios are Given/Then only).
	scenarioRe = regexp.MustCompile(`^\s*\d+\.\s+\*\*Given\*\*\s*(.+?)\s*(?:\*\*When\*\*\s*(.+?)\s*)?\*\*Then\*\*\s*(.+?)\s*$`)

	acceptanceHdrRe = regexp.MustCompile(`(?i)^\s*\*\*Acceptance Scenarios\*\*`)

	numberedRe = regexp.MustCompile(`^\s*\d+\.\s`)
)

// parseStories extracts user stories and their acceptance scenarios from one
// spec body. Each story heading (h3) opens a block that runs until the next h2/h3.
// roleUnparsed counts stories whose block contains an "I want" line that the
// narrative regex still failed to split — i.e. genuine parser misses, distinct
// from prose-style stories that carry no role narrative at all.
func parseStories(prefix, body string) (stories []importer.UserStory, scenarios []importer.Scenario, roleUnparsed int) {
	lines := strings.Split(body, "\n")

	for i := 0; i < len(lines); i++ {
		m := storyHeadingRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		position := atoiSafe(m[1])
		story := importer.UserStory{
			SpecPrefix: prefix,
			Position:   position,
			Title:      strings.TrimSpace(m[2]),
			Priority:   priorityNum(m[3]), // "P1" → 1 (req_priority level)
		}
		// Block extent: until the next h2/h3 heading.
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(lines[j], "## ") || strings.HasPrefix(lines[j], "### ") {
				end = j
				break
			}
		}
		block := lines[i+1 : end]
		blockHasWant := false

		inScenarios := false
		inLead := true
		var leadProse []string
		scnPosition := 0
		for bi := 0; bi < len(block); bi++ {
			ln := block[bi]
			// The lead ends at the first marker (Why/Independent/Acceptance), a
			// numbered scenario, or a sub-heading.
			tr := strings.TrimSpace(ln)
			if strings.HasPrefix(tr, "**") || numberedRe.MatchString(ln) || strings.HasPrefix(ln, "#") {
				inLead = false
			}
			if !blockHasWant && strings.Contains(strings.ToLower(ln), "i want") {
				blockHasWant = true
			}
			if story.IWant == "" && asAStartRe.MatchString(ln) {
				// The narrative may wrap across lines; join the continuation first so
				// the so_that isn't truncated, then match.
				joined := strings.TrimSpace(ln)
				cont, nbi := gatherWrapped(block, bi)
				if cont != "" {
					joined += " " + cont
				}
				if am := asARe.FindStringSubmatch(joined); am != nil {
					// The "As a …" narrative leaves the indefinite article on the captured
					// role; strip a leading "a " so as_a holds the bare role ("a tutor" →
					// "tutor"). "an …"/"the …" don't start with "a " and are left untouched.
					asA := strings.TrimSpace(am[1])
					if len(asA) >= 2 && strings.EqualFold(asA[:2], "a ") {
						asA = asA[2:]
					}
					story.AsA = asA
					story.IWant = strings.TrimSpace(am[2])
					story.SoThat = strings.TrimSpace(am[3])
					if cont != "" {
						bi = nbi
					}
				}
			}
			if m := whyPriorityRe.FindStringSubmatch(ln); m != nil {
				cont, nbi := gatherWrapped(block, bi)
				story.WhyPriority = joinWrapped(strings.TrimSpace(m[1]), cont)
				bi = nbi
				continue
			}
			if m := indepTestRe.FindStringSubmatch(ln); m != nil {
				cont, nbi := gatherWrapped(block, bi)
				story.IndependentTest = joinWrapped(strings.TrimSpace(m[1]), cont)
				bi = nbi
				continue
			}
			if acceptanceHdrRe.MatchString(ln) {
				inScenarios = true
				continue
			}
			if inLead {
				leadProse = append(leadProse, ln)
			}
			if !inScenarios || !numberedRe.MatchString(ln) {
				continue
			}
			// A numbered scenario may wrap across lines; gather its continuation
			// until the next numbered item, a blank line, or a heading.
			text := strings.TrimSpace(ln)
			for bi+1 < len(block) {
				nxt := block[bi+1]
				if strings.TrimSpace(nxt) == "" || numberedRe.MatchString(nxt) || strings.HasPrefix(nxt, "#") {
					break
				}
				text += " " + strings.TrimSpace(nxt)
				bi++
			}
			if sm := scenarioRe.FindStringSubmatch(text); sm != nil {
				scnPosition++
				scenarios = append(scenarios, importer.Scenario{
					SpecPrefix:    prefix,
					StoryPosition: position,
					Position:      scnPosition,
					Given:         cleanGWT(sm[1]),
					When:          cleanGWT(sm[2]),
					Then:          cleanGWT(sm[3]),
				})
			}
		}
		if story.IWant == "" && blockHasWant {
			roleUnparsed++
		}
		if story.IWant == "" {
			story.Narrative = strings.TrimSpace(strings.Join(leadProse, "\n"))
		}
		stories = append(stories, story)
		i = end - 1
	}
	return stories, scenarios, roleUnparsed
}

// cleanGWT trims a Given/When/Then fragment of trailing comma/space.
func cleanGWT(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), ", ")
}

// gatherWrapped joins the wrapped continuation of a marker line (e.g. a
// multi-line "Why this priority"), stopping at a blank line, the next bold
// marker, a numbered item, or a heading. Returns the joined text and the index
// of the last consumed line.
func gatherWrapped(block []string, bi int) (string, int) {
	var parts []string
	for bi+1 < len(block) {
		nxt := block[bi+1]
		t := strings.TrimSpace(nxt)
		if t == "" || strings.HasPrefix(t, "**") || numberedRe.MatchString(nxt) || strings.HasPrefix(nxt, "#") {
			break
		}
		parts = append(parts, t)
		bi++
	}
	return strings.Join(parts, " "), bi
}

// joinWrapped appends a continuation to a first line with a single space.
func joinWrapped(first, cont string) string {
	if cont == "" {
		return first
	}
	return first + " " + cont
}

func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// priorityNum maps a "P{n}" tag to its level (P1→1). The 0–4 scheme reserves 0
// (Critical) for security/data-loss/broken-build emergencies, which user stories
// don't carry — so the corpus's P1–P4 land on 1–4.
func priorityNum(tag string) int {
	return atoiSafe(strings.TrimPrefix(tag, "P"))
}
