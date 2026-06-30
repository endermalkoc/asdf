package tutor

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/endermalkoc/cusp/internal/enums"
	"github.com/endermalkoc/cusp/internal/importer"
)

// frStatement is the text parsed from a spec's bold FR line, to be joined onto
// the authoritative registry entry by fr_key.
type frStatement struct {
	Statement  string
	Section    string // FR group title it belongs to (link key)
	Position   int    // document order within the spec's FR list
	Tombstoned bool
	Delegated  bool // came from a "FR-x to FR-y" range line
	matched    bool // set true once a registry entry consumes it
}

// frGroupHeaderRe matches a standalone bold FR group sub-header line, e.g.
// "**Student Section**" or "**Family Section (Children Only)**" — a fully-bold
// line with no trailing text (so "**Acceptance Scenarios**:" does not match).
var frGroupHeaderRe = regexp.MustCompile(`^\s*\*\*(.+?)\*\*\s*$`)

// frHeadingGroupRe matches an h4+ heading used as an FR group sub-header inside
// the Functional Requirements list, e.g. "#### Brute-Force Protection (M4)" — the
// heading-style equivalent of a bold sub-header. h1–h3 are document structure
// ("# Title", "## Requirements", "### Functional Requirements") and are excluded.
var frHeadingGroupRe = regexp.MustCompile(`^\s*#{4,}\s+(.+?)\s*$`)

// frBoundary reports whether a line ends an FR body or group note: a new FR
// definition, a bold group sub-header, or any heading. Indented continuation and
// sub-bullets are NOT boundaries, so an FR's full multi-line body is captured.
func frBoundary(line string) bool {
	return frLineRe.MatchString(line) || frRangeLineRe.MatchString(line) ||
		frGroupHeaderRe.MatchString(line) || strings.HasPrefix(line, "#")
}

// gatherBody consumes lines[i:] until the next FR boundary, returning the joined
// (trimmed) continuation and the new index.
func gatherBody(lines []string, i int) (string, int) {
	var body []string
	for i < len(lines) && !frBoundary(lines[i]) {
		body = append(body, lines[i])
		i++
	}
	return strings.TrimRight(strings.TrimLeft(strings.Join(body, "\n"), "\n"), " \n"), i
}

// collectStatements parses the bold FR definition lines (and delegated ranges)
// out of one spec body into stmtByKey, keyed by fr_key. Each FR's full body — the
// first line plus any indented continuation / sub-bullets — is captured as its
// statement. It also assembles the FR groups: each bold sub-header opens a group,
// the prose between the header and the first FR becomes the group's note, and each
// FR records its group header and document position. Returns the groups in order.
func collectStatements(prefix, body string, stmtByKey map[string]frStatement, rep *importer.Report) ([]importer.ReqGroup, string) {
	lines := strings.Split(body, "\n")
	var groups []importer.ReqGroup
	moreInfo := ""
	addMore := func(block string) {
		if moreInfo == "" {
			moreInfo = block
		} else {
			moreInfo += "\n\n" + block
		}
	}
	currentHeader := ""
	pos := 0
	i := 0
	for i < len(lines) {
		line := lines[i]

		if strings.HasPrefix(line, "#") {
			// An h4+ heading immediately followed by an FR is an FR group sub-header
			// (the heading-style equivalent of a bold one) — promote it to a group;
			// h1–h3 and FR-less headings are document structure, so reset and skip.
			if m := frHeadingGroupRe.FindStringSubmatch(line); m != nil {
				header := strings.TrimSpace(m[1])
				body2, ni := gatherBody(lines, i+1)
				if ni < len(lines) && (frLineRe.MatchString(lines[ni]) || frRangeLineRe.MatchString(lines[ni])) {
					currentHeader = header
					groups = append(groups, importer.ReqGroup{Position: len(groups) + 1, Title: header, Notes: body2})
					i = ni
					continue
				}
			}
			currentHeader = ""
			i++
			continue
		}

		// A bold sub-header is a real FR group only if an FR follows it; otherwise
		// it is note-only content (e.g. "Column Configuration") → more_info.
		if m := frGroupHeaderRe.FindStringSubmatch(line); m != nil {
			header := strings.TrimSpace(m[1])
			body2, ni := gatherBody(lines, i+1)
			if ni < len(lines) && (frLineRe.MatchString(lines[ni]) || frRangeLineRe.MatchString(lines[ni])) {
				currentHeader = header
				groups = append(groups, importer.ReqGroup{Position: len(groups) + 1, Title: header, Notes: body2})
			} else {
				currentHeader = ""
				block := "**" + header + "**"
				if body2 != "" {
					block += "\n\n" + body2
				}
				addMore(block)
			}
			i = ni
			continue
		}

		// Delegated range: "- **P-FR-004 to P-FR-005**: text follows [shared/...]"
		if m := frRangeLineRe.FindStringSubmatch(line); m != nil {
			p1, n1, _, ok1 := splitFRKey(m[1] + "-FR-" + m[2] + m[3])
			p2, n2, _, ok2 := splitFRKey(m[4] + "-FR-" + m[5] + m[6])
			text := trimMarkdown(m[7])
			cont, ni := gatherBody(lines, i+1)
			if cont != "" {
				text += "\n" + cont
			}
			i = ni
			if !ok1 || !ok2 || p1 != p2 {
				rep.Add(importer.SevWarn, "bad-fr-range", "could not expand FR range: "+strings.TrimSpace(line), prefix)
				continue
			}
			for n := n1; n <= n2; n++ {
				key := frKey(p1, n, "")
				if _, exists := stmtByKey[key]; !exists {
					stmtByKey[key] = frStatement{Statement: text, Section: currentHeader, Position: pos, Delegated: true}
				}
				pos++
			}
			continue
		}

		// Single bold FR line. Any [visual]/[operational]/… marker is parsed off the
		// statement text and discarded (the opt-out concept was dropped).
		if m := frLineRe.FindStringSubmatch(line); m != nil {
			prefixID, numStr, suffix, text := m[1], m[2], m[3], trimMarkdown(m[5])
			cont, ni := gatherBody(lines, i+1)
			i = ni
			_, n, _, ok := splitFRKey(prefixID + "-FR-" + numStr + suffix)
			if !ok {
				continue
			}
			if cont != "" {
				text += "\n" + cont
			}
			key := frKey(prefixID, n, suffix)
			tomb := strings.HasPrefix(text, "_(") || strings.HasPrefix(text, "*(")
			st := frStatement{Statement: text, Section: currentHeader, Position: pos, Tombstoned: tomb}
			pos++
			if _, exists := stmtByKey[key]; exists {
				rep.Add(importer.SevWarn, "duplicate-fr-line", "fr_key defined by more than one spec line", key)
			}
			stmtByKey[key] = st
			continue
		}

		i++
	}
	return groups, moreInfo
}

// regEntry is one fr-registry/**.yml value.
type regEntry struct {
	Status    string `yaml:"status"`
	Milestone string `yaml:"milestone"`
	Notes     string `yaml:"notes"`
	E2ERef    string `yaml:"e2e_ref"`
}

// parseRegistry walks fr-registry/**.yml — the authoritative requirement set —
// and joins each entry to its statement in stmtByKey. It returns the
// requirements and the distinct milestones, recording summary findings into rep.
func parseRegistry(registryDir string, stmtByKey map[string]frStatement, rep *importer.Report) ([]importer.Requirement, []importer.Milestone, error) {
	var reqs []importer.Requirement
	milestoneSet := map[string]bool{}
	var (
		noStatement   int
		e2eRefCount   int
		badKeys       int
		unknownStatus = map[string]bool{}
	)

	if _, err := os.Stat(registryDir); err != nil {
		rep.Add(importer.SevWarn, "no-registry", "fr-registry/ not found; requirements left empty", registryDir)
		return nil, nil, nil
	}

	err := filepath.WalkDir(registryDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".yml" {
			return nil
		}
		b, readErr := os.ReadFile(p)
		if readErr != nil {
			return nil
		}
		var entries map[string]regEntry
		if e := yaml.Unmarshal(b, &entries); e != nil {
			rep.Add(importer.SevWarn, "bad-registry-yaml", "could not parse registry file: "+e.Error(), p)
			return nil
		}
		for key, ent := range entries {
			prefix, number, suffix, ok := splitFRKey(key)
			if !ok {
				badKeys++
				rep.Add(importer.SevWarn, "bad-fr-key", "registry key is not a valid FR id", key)
				continue
			}
			if ent.Status != "" && !enums.Valid(enums.RequirementDelivery, ent.Status) {
				unknownStatus[ent.Status] = true
			}
			st, hasStmt := stmtByKey[key]
			if hasStmt {
				st.matched = true
				stmtByKey[key] = st
			} else {
				noStatement++
			}
			if ent.E2ERef != "" {
				e2eRefCount++
			}
			if ent.Milestone != "" {
				milestoneSet[ent.Milestone] = true
			}
			reqs = append(reqs, importer.Requirement{
				FRKey:          key,
				SpecPrefix:     prefix,
				Number:         number,
				Suffix:         suffix,
				Statement:      st.Statement,
				DeliveryStatus: ent.Status,
				Milestone:      ent.Milestone,
				E2ERef:         ent.E2ERef,
				Section:        st.Section,
				Position:       st.Position,
				Notes:          ent.Notes,
				Tombstoned:     st.Tombstoned,
			})
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	sort.Slice(reqs, func(i, j int) bool {
		if reqs[i].SpecPrefix != reqs[j].SpecPrefix {
			return reqs[i].SpecPrefix < reqs[j].SpecPrefix
		}
		if reqs[i].Number != reqs[j].Number {
			return reqs[i].Number < reqs[j].Number
		}
		return reqs[i].Suffix < reqs[j].Suffix
	})

	var milestones []importer.Milestone
	for m := range milestoneSet {
		milestones = append(milestones, importer.Milestone{Slug: m})
	}
	sort.Slice(milestones, func(i, j int) bool { return milestones[i].Slug < milestones[j].Slug })

	// Summary findings (one each, not one per row).
	if noStatement > 0 {
		rep.Add(importer.SevInfo, "registry-no-statement",
			itoa(noStatement)+" registry FRs have no statement line in their spec (delegated/shared or drift)", "")
	}
	if e2eRefCount > 0 {
		rep.Add(importer.SevInfo, "registry-e2e-ref",
			itoa(e2eRefCount)+" registry entries set e2e_ref → requirement.notes for now (test_case.path once a test source is imported)", "")
	}
	for s := range unknownStatus {
		rep.Add(importer.SevWarn, "unknown-delivery-status",
			"registry status "+s+" is outside the delivery_status seed set — tolerated and stored (add a row to the delivery_status table to make it first-class)", "")
	}

	return reqs, milestones, nil
}

// itoa avoids importing strconv just for report messages.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
