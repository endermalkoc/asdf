package importer

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/endermalkoc/asdf/internal/store"
)

// TouchedTables lists every table Apply may write, so the caller (the Mutate
// wrapper) can stage them for the Dolt commit.
var TouchedTables = []string{
	"req_domain", "plan_milestone", "req_spec", "req_requirement", "req_requirement_group",
	"req_user_story", "req_acceptance_scenario", "req_entity_ref", "pub_external_ref",
	"ent_entity", "req_doc_section",
}

// ApplyStats tallies the write, per entity kind.
type ApplyStats struct {
	Inserted map[string]int `json:"inserted"`
	Updated  map[string]int `json:"updated"`
	Skipped  map[string]int `json:"skipped"`
}

func newStats() *ApplyStats {
	return &ApplyStats{Inserted: map[string]int{}, Updated: map[string]int{}, Skipped: map[string]int{}}
}

func (s *ApplyStats) bump(kind string, inserted bool) {
	if inserted {
		s.Inserted[kind]++
	} else {
		s.Updated[kind]++
	}
}

// Apply writes a staged Graph into the database through x (the Mutate wrapper's
// transaction), in dependency order, idempotently. It applies the import mapping
// decisions recorded in docs/entities/decisions.md:
//   - a requirement's e2e_ref is folded into notes (test linkage has no column yet);
//   - a milestone value that is a beads issue id (tut-*) becomes an ExternalRef on
//     the requirement, not a Milestone;
//   - every other milestone value (incl. backlog) is a real Milestone.
//
// Rows whose parent could not be resolved (e.g. a requirement for an unregistered
// spec) are skipped and counted, never silently dropped.
func Apply(ctx context.Context, x store.Execer, g *Graph) (*ApplyStats, error) {
	st := newStats()

	// Domains.
	domainID := map[string]string{}
	for _, d := range g.Domains {
		id, ins, err := store.UpsertDomain(ctx, x, store.Domain{
			Abbreviation: d.Abbrev, Name: d.Name, Description: d.Description, Kind: d.Kind,
		})
		if err != nil {
			return st, err
		}
		domainID[d.Abbrev] = id
		st.bump("domains", ins)
	}

	// Milestones (skip beads issue ids — those become external refs below).
	milestoneID := map[string]string{}
	for _, m := range g.Milestones {
		if isIssueID(m.Abbrev) {
			continue
		}
		id, ins, err := store.UpsertMilestone(ctx, x, store.Milestone{Abbreviation: m.Abbrev})
		if err != nil {
			return st, err
		}
		milestoneID[m.Abbrev] = id
		st.bump("milestones", ins)
	}

	// Specs (registry + entity-doc specs). Indexed by prefix and by path.
	specID := map[string]string{}     // prefix → id
	specByPath := map[string]string{} // path → id
	for _, sp := range g.Specs {
		dID, ok := domainID[sp.Domain]
		if !ok {
			st.Skipped["specs"]++
			continue
		}
		id, ins, err := store.UpsertSpec(ctx, x, dID, store.Spec{
			Prefix: sp.Prefix, Slug: slugFromPath(sp.Path), Path: sp.Path,
			Title: sp.Title, Kind: sp.Kind, Status: sp.Status, Heading: sp.Heading,
		})
		if err != nil {
			return st, err
		}
		if sp.Prefix != "" {
			specID[sp.Prefix] = id
		}
		specByPath[sp.Path] = id
		st.bump("specs", ins)
		// Reconcile the owner's sections (delete-then-insert) so a changed parse
		// leaves no orphans, then write keyed + bespoke sections.
		if e := store.DeleteDocSectionsByOwner(ctx, x, "spec", id); e != nil {
			return st, e
		}
		for _, ds := range sp.Sections {
			_, dins, e := store.UpsertDocSection(ctx, x, "spec", id, ds.Ordinal, ds.Level, ds.Heading, ds.Body, ds.Key)
			if e != nil {
				return st, e
			}
			st.bump("doc_sections", dins)
		}
	}

	// FR groups (before requirements, so group_id resolves). Keyed (specID, header).
	groupID := map[string]string{}
	for _, sp := range g.Specs {
		sid, ok := specID[sp.Prefix]
		if !ok {
			continue
		}
		for _, gr := range sp.ReqGroups {
			id, ins, err := store.UpsertRequirementGroup(ctx, x, sid, gr.Position, gr.Header, gr.Note)
			if err != nil {
				return st, err
			}
			groupID[sid+"\x1f"+gr.Header] = id
			st.bump("requirement_groups", ins)
		}
	}

	// Requirements (+ collect beads-issue milestones for external refs).
	reqID := map[string]string{} // fr_key → id
	type extRef struct{ subjectID, externalID string }
	var extRefs []extRef
	for _, r := range g.Reqs {
		sid, ok := specID[r.SpecPrefix]
		if !ok {
			st.Skipped["requirements"]++
			continue
		}
		msID := ""
		if r.Milestone != "" && !isIssueID(r.Milestone) {
			msID = milestoneID[r.Milestone] // "" if the milestone was not created
		}
		contentStatus := "active"
		if r.Tombstoned {
			contentStatus = "obsolete"
		}
		gid := ""
		if r.Section != "" {
			gid = groupID[sid+"\x1f"+r.Section]
		}
		id, ins, err := store.UpsertRequirement(ctx, x, sid, store.Requirement{
			Number: r.Number, Suffix: r.Suffix, FRKey: r.FRKey, Statement: r.Statement,
			ContentStatus: contentStatus, DeliveryStatus: r.DeliveryStatus, MilestoneID: msID,
			Owner: r.Owner, Notes: foldE2E(r.Notes, r.E2ERef), OptoutMarker: r.OptoutMarker,
			GroupID: gid, Position: r.Position,
		})
		if err != nil {
			return st, err
		}
		reqID[r.FRKey] = id
		st.bump("requirements", ins)
		if r.Milestone != "" && isIssueID(r.Milestone) {
			extRefs = append(extRefs, extRef{subjectID: id, externalID: r.Milestone})
		}
	}

	// User stories.
	storyID := map[string]string{} // prefix#ordinal → id
	for _, us := range g.Stories {
		sid, ok := specID[us.SpecPrefix]
		if !ok {
			st.Skipped["user_stories"]++
			continue
		}
		id, ins, err := store.UpsertUserStory(ctx, x, store.UserStory{
			SpecID: sid, Ordinal: us.Ordinal, Title: us.Title, Priority: us.Priority,
			AsA: us.AsA, IWant: us.IWant, SoThat: us.SoThat, Narrative: us.Narrative,
			WhyPriority: us.WhyPriority, IndependentTest: us.IndependentTest,
		})
		if err != nil {
			return st, err
		}
		storyID[storyKey(us.SpecPrefix, us.Ordinal)] = id
		st.bump("user_stories", ins)
	}

	// Acceptance scenarios.
	for _, sc := range g.Scenarios {
		usid, ok := storyID[storyKey(sc.SpecPrefix, sc.StoryOrdinal)]
		if !ok {
			st.Skipped["acceptance_scenarios"]++
			continue
		}
		_, ins, err := store.UpsertScenario(ctx, x, store.Scenario{
			UserStoryID: usid, Ordinal: sc.Ordinal, Given: sc.Given, When: sc.When, Then: sc.Then,
		})
		if err != nil {
			return st, err
		}
		st.bump("acceptance_scenarios", ins)
	}

	// Entities (glossary) → entity rows + their doc sections. Done before edges so
	// "Key Entities" spec→entity edges can resolve the entity ids.
	entitiesDomainID := domainID["entities"]
	entityID := map[string]string{} // name → id
	for _, e := range g.Entities {
		if entitiesDomainID == "" {
			st.Skipped["entities"]++
			continue
		}
		id, ins, err := store.UpsertEntity(ctx, x, entitiesDomainID, specByPath[e.DocPath], store.Entity{
			Name: e.Name, Description: e.Description, Status: e.Status,
		})
		if err != nil {
			return st, err
		}
		entityID[e.Name] = id
		st.bump("entities", ins)
		if e2 := store.DeleteDocSectionsByOwner(ctx, x, "entity", id); e2 != nil {
			return st, e2
		}
		for _, ds := range e.Sections {
			_, dins, err := store.UpsertDocSection(ctx, x, "entity", id, ds.Ordinal, ds.Level, ds.Heading, ds.Body, ds.Key)
			if err != nil {
				return st, err
			}
			st.bump("doc_sections", dins)
		}
	}

	// EntityRefs (prose-derived cross-references). Resolve each endpoint by its type:
	// requirement→reqID(fr_key), spec→specByPath(path), entity→entityID(name),
	// domain→domainID(abbrev), milestone→milestoneID(abbrev).
	resolve := func(typ, key string) (string, bool) {
		switch typ {
		case "requirement":
			id, ok := reqID[key]
			return id, ok
		case "spec":
			id, ok := specByPath[key]
			return id, ok
		case "entity":
			id, ok := entityID[key]
			return id, ok
		case "domain":
			id, ok := domainID[key]
			return id, ok
		case "milestone":
			id, ok := milestoneID[key]
			return id, ok
		}
		return "", false
	}
	for _, r := range g.Refs {
		ownerID, ok1 := resolve(r.OwnerType, r.OwnerKey)
		targetID, ok2 := resolve(r.TargetType, r.TargetKey)
		if !ok1 || !ok2 {
			st.Skipped["entity_refs"]++
			continue
		}
		if _, err := store.UpsertEntityRef(ctx, x, r.OwnerType, ownerID, r.TargetType, targetID, r.Kind); err != nil {
			return st, err
		}
		st.Inserted["entity_refs"]++ // INSERT IGNORE: counted as processed
	}

	// External refs: beads issue ids that were used as milestones.
	for _, er := range extRefs {
		_, ins, err := store.UpsertExternalRef(ctx, x, "requirement", er.subjectID, "beads", er.externalID, "")
		if err != nil {
			return st, err
		}
		st.bump("external_refs", ins)
	}

	return st, nil
}

// isIssueID reports whether a milestone value is actually a beads issue id
// (project prefix `tut-`), not a milestone label.
func isIssueID(s string) bool {
	return strings.HasPrefix(strings.ToLower(s), "tut-")
}

// foldE2E appends an e2e test reference to notes (test linkage has no column yet).
func foldE2E(notes, e2e string) string {
	e2e = strings.TrimSpace(e2e)
	if e2e == "" {
		return notes
	}
	tag := "e2e: " + e2e
	if strings.TrimSpace(notes) == "" {
		return tag
	}
	return notes + " [" + tag + "]"
}

func slugFromPath(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

func storyKey(prefix string, ordinal int) string {
	return prefix + "#" + strconv.Itoa(ordinal)
}
