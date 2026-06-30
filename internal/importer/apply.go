package importer

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/endermalkoc/cusp/internal/ids"
	"github.com/endermalkoc/cusp/internal/store"
)

// TouchedTables lists every table Apply may write, so the caller (the Mutate
// wrapper) can stage them for the Dolt commit.
var TouchedTables = []string{
	"req_domain", "plan_milestone", "req_spec", "req_requirement", "req_requirement_group",
	"req_user_story", "req_acceptance_scenario", "req_entity_ref", "pub_external_ref",
	"ent_entity", "req_spec_section", "ent_entity_section", "ent_relationship",
	// planning layer (Notion adapter)
	"plan_capability", "plan_deliverable", "plan_view",
	"plan_capability_milestone", "plan_capability_deliverable",
	"plan_deliverable_view", "plan_deliverable_dependency",
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

// sectionTypeKeySet loads a curated section-type vocabulary into a key set.
func sectionTypeKeySet(ctx context.Context, x store.Execer, list func(context.Context, store.Execer) ([]store.SectionTypeRow, error)) (map[string]bool, error) {
	rows, err := list(ctx, x)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, t := range rows {
		set[t.Key] = true
	}
	return set, nil
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

	// Curated section-type vocabularies (seeded by migration). A section whose key the
	// seed does not know is skipped and counted — so importer↔seed drift surfaces in the
	// report rather than corrupting data.
	specSectionKeys, err := sectionTypeKeySet(ctx, x, store.ListSpecSectionTypes)
	if err != nil {
		return st, err
	}
	entitySectionKeys, err := sectionTypeKeySet(ctx, x, store.ListEntitySectionTypes)
	if err != nil {
		return st, err
	}

	// Domains.
	domainID := map[string]string{}
	for _, d := range g.Domains {
		id, ins, err := store.UpsertDomain(ctx, x, store.Domain{
			Slug: d.Slug, Name: d.Name, Description: d.Description,
		})
		if err != nil {
			return st, err
		}
		domainID[d.Slug] = id
		st.bump("domains", ins)
	}

	// Milestones (skip beads issue ids — those become external refs below).
	milestoneID := map[string]string{}
	for _, m := range g.Milestones {
		if isIssueID(m.Slug) {
			continue
		}
		id, ins, err := store.UpsertMilestone(ctx, x, store.Milestone{Slug: m.Slug})
		if err != nil {
			return st, err
		}
		milestoneID[m.Slug] = id
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
		// Stored path is domain-relative (the domain is domain_id; full = domain.slug +
		// "/" + path, migration 0017). specByPath stays keyed by the full source path,
		// which is how the importer's cross-references address specs.
		id, ins, err := store.UpsertSpec(ctx, x, dID, store.Spec{
			Prefix: sp.Prefix, Slug: slugFromPath(sp.Path), Path: domainRelDir(sp.Path),
			Title: sp.Title, Status: sp.Status, CreatedAt: sp.Created,
		})
		if err != nil {
			return st, err
		}
		if sp.Prefix != "" {
			specID[sp.Prefix] = id
		}
		specByPath[sp.Path] = id
		st.bump("specs", ins)
		// Reconcile the spec's sections (delete-then-insert) so a changed parse leaves
		// no orphans. A key the seed does not know is skipped + counted.
		if e := store.DeleteSpecSectionsBySpec(ctx, x, id); e != nil {
			return st, e
		}
		for _, ds := range sp.Sections {
			if !specSectionKeys[ds.Key] {
				st.Skipped["sections"]++
				continue
			}
			_, dins, e := store.UpsertSpecSection(ctx, x, id, ds.Key, ds.Body)
			if e != nil {
				return st, e
			}
			st.bump("sections", dins)
		}
	}

	// FR groups (before requirements, so group_id resolves). Keyed (specID, title).
	groupID := map[string]string{}
	for _, sp := range g.Specs {
		sid, ok := specID[sp.Prefix]
		if !ok {
			continue
		}
		for _, gr := range sp.ReqGroups {
			id, ins, err := store.UpsertRequirementGroup(ctx, x, sid, gr.Position, gr.Title, gr.Notes)
			if err != nil {
				return st, err
			}
			groupID[sid+"\x1f"+gr.Title] = id
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
			Notes:   foldE2E(r.Notes, r.E2ERef),
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
	storyID := map[string]string{} // prefix#position → id
	for _, us := range g.Stories {
		sid, ok := specID[us.SpecPrefix]
		if !ok {
			st.Skipped["user_stories"]++
			continue
		}
		id, ins, err := store.UpsertUserStory(ctx, x, store.UserStory{
			SpecID: sid, Position: us.Position, Title: us.Title, Priority: us.Priority,
			AsA: us.AsA, IWant: us.IWant, SoThat: us.SoThat, Narrative: us.Narrative,
			WhyPriority: us.WhyPriority, IndependentTest: us.IndependentTest,
		})
		if err != nil {
			return st, err
		}
		storyID[storyKey(us.SpecPrefix, us.Position)] = id
		st.bump("user_stories", ins)
	}

	// Acceptance scenarios.
	for _, sc := range g.Scenarios {
		usid, ok := storyID[storyKey(sc.SpecPrefix, sc.StoryPosition)]
		if !ok {
			st.Skipped["acceptance_scenarios"]++
			continue
		}
		_, ins, err := store.UpsertScenario(ctx, x, store.Scenario{
			UserStoryID: usid, Position: sc.Position, Given: sc.Given, When: sc.When, Then: sc.Then,
		})
		if err != nil {
			return st, err
		}
		st.bump("acceptance_scenarios", ins)
	}

	// Entities (glossary) → entity rows + their doc sections. Done before edges so
	// "Key Entities" spec→entity edges can resolve the entity ids. Entities are
	// domain-less first-class documents; path is the full doc path.
	entityID := map[string]string{} // name → id
	for _, e := range g.Entities {
		id, ins, err := store.UpsertEntity(ctx, x, store.Entity{
			Name: e.Name, Path: domainRelDir(e.DocPath), Description: e.Description, Status: e.Status,
		})
		if err != nil {
			return st, err
		}
		entityID[e.Name] = id
		st.bump("entities", ins)
		if e2 := store.DeleteEntitySectionsByEntity(ctx, x, id); e2 != nil {
			return st, e2
		}
		for _, ds := range e.Sections {
			if !entitySectionKeys[ds.Key] {
				st.Skipped["sections"]++
				continue
			}
			_, dins, err := store.UpsertEntitySection(ctx, x, id, ds.Key, ds.Body)
			if err != nil {
				return st, err
			}
			st.bump("sections", dins)
		}
	}

	// Entity relationships (from the Drizzle schema). Resolve both endpoints by name.
	for _, r := range g.Relationships {
		fromID, ok1 := entityID[r.FromName]
		toID, ok2 := entityID[r.ToName]
		if !ok1 || !ok2 {
			st.Skipped["relationships"]++
			continue
		}
		if _, err := store.UpsertEntityRelationship(ctx, x, fromID, toID, r.Cardinality, r.JunctionTable); err != nil {
			return st, err
		}
		st.Inserted["relationships"]++ // INSERT IGNORE: counted as processed
	}

	// EntityRefs (prose-derived cross-references). Resolve each endpoint by its type:
	// requirement→reqID(fr_key), spec→specByPath(path), entity→entityID(name),
	// domain→domainID(slug), milestone→milestoneID(slug).
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
		if _, err := store.UpsertEntityRef(ctx, x, r.OwnerType, ownerID, r.TargetType, targetID); err != nil {
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

	if err := applyPlanning(ctx, x, g, st, domainID, milestoneID); err != nil {
		return st, err
	}

	return st, nil
}

// applyPlanning writes the planning layer (Capability/Deliverable/View + junctions
// + deliverable external refs). Rows are keyed by an external source id (e.g. a Notion
// page id); the row id is derived deterministically (ids.Rel) so re-import converges.
// Junctions are reconciled per owner (clear-then-link), so a relation removed at the
// source drops its row on re-import. domainID/milestoneID are the resolved id maps the
// caller already built (the adapter seeds g.Domains/g.Milestones for every value it uses).
func applyPlanning(ctx context.Context, x store.Execer, g *Graph, st *ApplyStats, domainID, milestoneID map[string]string) error {
	if len(g.Capabilities) == 0 && len(g.Deliverables) == 0 && len(g.Views) == 0 {
		return nil
	}
	capRowID := func(src string) string { return ids.Rel("plan_capability", src) }
	delivRowID := func(src string) string { return ids.Rel("plan_deliverable", src) }
	viewRowID := func(src string) string { return ids.Rel("plan_view", src) }

	// *OK tracks which source rows were actually inserted, so a junction never
	// references a row skipped for an unresolved domain (which would break an FK).
	capOK := map[string]bool{}
	delivOK := map[string]bool{}
	viewOK := map[string]bool{}

	for _, c := range g.Capabilities {
		dID, ok := domainID[c.DomainSlug]
		if !ok {
			st.Skipped["capabilities"]++
			continue
		}
		ins, err := store.UpsertCapability(ctx, x, store.Capability{
			ID: capRowID(c.SourceID), Title: c.Title, Level: c.Level, DomainID: dID,
		})
		if err != nil {
			return err
		}
		capOK[c.SourceID] = true
		st.bump("capabilities", ins)
		if c.SourceID != "" {
			_, eins, err := store.UpsertExternalRef(ctx, x, "capability", capRowID(c.SourceID), "notion", c.SourceID, c.SourceURL)
			if err != nil {
				return err
			}
			st.bump("external_refs", eins)
		}
	}
	for _, d := range g.Deliverables {
		ins, err := store.UpsertDeliverable(ctx, x, store.Deliverable{
			ID: delivRowID(d.SourceID), Title: d.Title, Size: d.Size, Status: d.Status,
			AIReady: d.AIReady, MilestoneID: milestoneID[d.MilestoneSlug],
		})
		if err != nil {
			return err
		}
		delivOK[d.SourceID] = true
		st.bump("deliverables", ins)
	}
	for _, v := range g.Views {
		dID, ok := domainID[v.DomainSlug]
		if !ok {
			st.Skipped["views"]++
			continue
		}
		specID := ""
		if v.SpecFile != "" {
			sid, found, err := store.FindSpecIDBySlug(ctx, x, slugFromPath(v.SpecFile))
			if err != nil {
				return err
			}
			if found {
				specID = sid
			}
		}
		ins, err := store.UpsertView(ctx, x, store.View{
			ID: viewRowID(v.SourceID), Title: v.Title, Route: v.Route, SpecID: specID, DomainID: dID,
		})
		if err != nil {
			return err
		}
		viewOK[v.SourceID] = true
		st.bump("views", ins)
		if v.SourceID != "" {
			_, eins, err := store.UpsertExternalRef(ctx, x, "view", viewRowID(v.SourceID), "notion", v.SourceID, v.SourceURL)
			if err != nil {
				return err
			}
			st.bump("external_refs", eins)
		}
	}

	// Capability parents (every capability row now exists).
	for _, c := range g.Capabilities {
		if !capOK[c.SourceID] {
			continue
		}
		parentID := ""
		if capOK[c.ParentSourceID] {
			parentID = capRowID(c.ParentSourceID)
		}
		if err := store.SetCapabilityParent(ctx, x, capRowID(c.SourceID), parentID); err != nil {
			return err
		}
	}

	// capability_milestone (owner = capability).
	for _, c := range g.Capabilities {
		if !capOK[c.SourceID] {
			continue
		}
		cid := capRowID(c.SourceID)
		if err := store.ClearCapabilityMilestones(ctx, x, cid); err != nil {
			return err
		}
		for _, ms := range c.MilestoneSlugs {
			if mid := milestoneID[ms]; mid != "" {
				if err := store.LinkCapabilityMilestone(ctx, x, cid, mid); err != nil {
					return err
				}
				st.Inserted["capability_milestone"]++
			}
		}
	}

	// capability_deliverable (union of both relation directions; owner = capability).
	capDelivs := map[string]map[string]bool{}
	addCD := func(capSrc, delivSrc string) {
		if !capOK[capSrc] || !delivOK[delivSrc] {
			return
		}
		if capDelivs[capSrc] == nil {
			capDelivs[capSrc] = map[string]bool{}
		}
		capDelivs[capSrc][delivSrc] = true
	}
	for _, c := range g.Capabilities {
		for _, ds := range c.DeliverableSourceIDs {
			addCD(c.SourceID, ds)
		}
	}
	for _, d := range g.Deliverables {
		for _, cs := range d.CapabilitySourceIDs {
			addCD(cs, d.SourceID)
		}
	}
	for _, c := range g.Capabilities {
		if !capOK[c.SourceID] {
			continue
		}
		cid := capRowID(c.SourceID)
		if err := store.ClearCapabilityDeliverables(ctx, x, cid); err != nil {
			return err
		}
		for ds := range capDelivs[c.SourceID] {
			if err := store.LinkCapabilityDeliverable(ctx, x, cid, delivRowID(ds)); err != nil {
				return err
			}
			st.Inserted["capability_deliverable"]++
		}
	}

	// deliverable_view (union of both relation directions; owner = deliverable).
	delivViews := map[string]map[string]bool{}
	addDV := func(delivSrc, viewSrc string) {
		if !delivOK[delivSrc] || !viewOK[viewSrc] {
			return
		}
		if delivViews[delivSrc] == nil {
			delivViews[delivSrc] = map[string]bool{}
		}
		delivViews[delivSrc][viewSrc] = true
	}
	for _, d := range g.Deliverables {
		for _, vs := range d.ViewSourceIDs {
			addDV(d.SourceID, vs)
		}
	}
	for _, v := range g.Views {
		for _, ds := range v.DeliverableSourceIDs {
			addDV(ds, v.SourceID)
		}
	}
	for _, d := range g.Deliverables {
		did := delivRowID(d.SourceID)
		if err := store.ClearDeliverableViews(ctx, x, did); err != nil {
			return err
		}
		for vs := range delivViews[d.SourceID] {
			if err := store.LinkDeliverableView(ctx, x, did, viewRowID(vs)); err != nil {
				return err
			}
			st.Inserted["deliverable_view"]++
		}
	}

	// deliverable_dependency (owner = deliverable; "blocked by").
	for _, d := range g.Deliverables {
		did := delivRowID(d.SourceID)
		if err := store.ClearDeliverableDependencies(ctx, x, did); err != nil {
			return err
		}
		for _, bs := range d.BlockedBySourceIDs {
			if delivOK[bs] {
				if err := store.LinkDeliverableDependency(ctx, x, did, delivRowID(bs)); err != nil {
					return err
				}
				st.Inserted["deliverable_dependency"]++
			}
		}
	}

	// External refs on deliverables: the Notion source (id + url) and any Bead IDs.
	for _, d := range g.Deliverables {
		did := delivRowID(d.SourceID)
		if d.SourceID != "" {
			_, ins, err := store.UpsertExternalRef(ctx, x, "deliverable", did, "notion", d.SourceID, d.SourceURL)
			if err != nil {
				return err
			}
			st.bump("external_refs", ins)
		}
		if bead := strings.TrimSpace(d.BeadIDs); bead != "" {
			_, ins, err := store.UpsertExternalRef(ctx, x, "deliverable", did, "beads", bead, "")
			if err != nil {
				return err
			}
			st.bump("external_refs", ins)
		}
	}
	return nil
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

// domainRelPath drops the leading segment (the domain directory) from a full docs path,
// so req_spec.path is stored relative to its domain (full = domain.slug + "/" + this;
// migration 0017). The top-level directory therefore always equals the domain — a spec
// filed under a different directory than its domain tag relocates under its domain.
func domainRelPath(p string) string {
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// domainRelDir is the DIRECTORY portion of the domain-relative path (no leading domain
// segment, no filename); "" for a top-level doc. req_spec/ent_entity store this, and
// reconstruct the filename from slug / kebab(name).
func domainRelDir(p string) string {
	if d := filepath.Dir(domainRelPath(p)); d != "." {
		return d
	}
	return ""
}

func storyKey(prefix string, position int) string {
	return prefix + "#" + strconv.Itoa(position)
}
