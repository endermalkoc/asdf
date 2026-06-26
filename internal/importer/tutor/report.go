package tutor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/endermalkoc/asdf/internal/importer"
)

// buildReport fills per-entity counts, the delivery-status histogram, and the
// cross-part findings that need the whole graph (orphans, ER gaps, milestone and
// status normalization).
func buildReport(g *importer.Graph, stmtByKey map[string]frStatement, rep *importer.Report) {
	rep.Counts["domains"] = len(g.Domains)
	rep.Counts["specs"] = len(g.Specs)
	rep.Counts["requirements"] = len(g.Reqs)
	rep.Counts["user_stories"] = len(g.Stories)
	rep.Counts["acceptance_scenarios"] = len(g.Scenarios)
	rep.Counts["entity_refs"] = len(g.Refs)
	rep.Counts["milestones"] = len(g.Milestones)
	rep.Counts["entities"] = len(g.Entities)

	// Entity layer: attributes/relationships stay deferred — they live in
	// entity-doc prose, not a structured form here.
	if len(g.Entities) > 0 {
		rep.Add(importer.SevInfo, "entity-attributes-deferred",
			"entity attributes/relationships are prose in the entity docs — not extracted (EntityAttribute = domain meaning, decisions.md)", "")
	}

	// Delivery-status histogram + optout/tombstone tallies.
	optout, tombstoned, noStmt := 0, 0, 0
	for _, r := range g.Reqs {
		status := r.DeliveryStatus
		if status == "" {
			status = "(unset)"
		}
		rep.Coverage[status]++
		if r.OptoutMarker != "" {
			optout++
		}
		if r.Tombstoned {
			tombstoned++
		}
		if r.Statement == "" {
			noStmt++
		}
	}
	if optout > 0 {
		rep.Add(importer.SevInfo, "optout-marker",
			itoa(optout)+" requirements carry a [visual]/[operational]/[untestable] marker (operational→ops mapping)", "")
	}
	if tombstoned > 0 {
		rep.Add(importer.SevInfo, "tombstoned-fr",
			itoa(tombstoned)+" requirements are tombstoned (intentionally omitted) in their spec", "")
	}

	// Orphan statements: spec FR lines with no registry entry.
	orphans := 0
	for _, st := range stmtByKey {
		if !st.matched && !st.Tombstoned {
			orphans++
		}
	}
	if orphans > 0 {
		rep.Add(importer.SevWarn, "orphan-fr-line",
			itoa(orphans)+" bold FR lines in specs have no fr-registry entry (definition without coverage row)", "")
	}

	// Resolved (decisions.md): spec status "Reviewed" now maps to spec.status 'reviewed'.
	for _, sp := range g.Specs {
		if strings.EqualFold(sp.RawStatus, "reviewed") {
			rep.Add(importer.SevInfo, "spec-status-reviewed",
				"spec status 'Reviewed' → spec.status 'reviewed' (added to the enum)", "")
			break
		}
	}

	// Resolved (migration 0002): domain descriptions now have a home.
	for _, d := range g.Domains {
		if strings.TrimSpace(d.Description) != "" {
			rep.Add(importer.SevInfo, "domain-description",
				"index.md domain descriptions → domain.description (added in 0002)", "")
			break
		}
	}

	// Milestone classification. tut-* values are beads issue ids → external_ref
	// (resolved); backlog is a valid open-string milestone value (resolved).
	for _, m := range g.Milestones {
		if knownMilestone(m.Abbrev) {
			continue
		}
		if strings.HasPrefix(m.Abbrev, "tut-") {
			rep.Add(importer.SevInfo, "milestone-is-issue-id",
				"milestone "+m.Abbrev+" is a beads issue id → external_ref(system=beads), not a milestone", "")
		} else {
			rep.Add(importer.SevInfo, "extra-milestone",
				"milestone value "+m.Abbrev+" is outside M0..M7/Future (valid open-string value, e.g. backlog)", "")
		}
	}

	// (Story-narrative classification — genuine misses vs prose-style — is emitted
	// by Parse, which has the per-block "I want" signal.)
}

func knownMilestone(abbrev string) bool {
	switch strings.ToLower(abbrev) {
	case "m0", "m1", "m2", "m3", "m4", "m5", "m6", "m7", "future":
		return true
	}
	return false
}

// crossCheckCoverage compares the parsed requirement count against the corpus's
// own fr-coverage-summary.json (if present) — a quick integrity signal.
func crossCheckCoverage(docsRoot string, g *importer.Graph, rep *importer.Report) {
	b, err := os.ReadFile(filepath.Join(docsRoot, "fr-coverage-summary.json"))
	if err != nil {
		return
	}
	var summary struct {
		Total int `json:"total"`
	}
	if json.Unmarshal(b, &summary) != nil || summary.Total == 0 {
		return
	}
	if summary.Total != len(g.Reqs) {
		rep.Add(importer.SevInfo, "coverage-count-drift",
			"parsed "+itoa(len(g.Reqs))+" requirements; fr-coverage-summary.json reports "+itoa(summary.Total), "")
	}
}
