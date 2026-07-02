package importer

import (
	"context"
	"testing"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/store"
	"github.com/endermalkoc/cusp/internal/testutil"
	"github.com/endermalkoc/cusp/internal/workspace"
)

// pi returns a pointer to an int literal (for the nullable *int fields).
func pi(n int) *int { return &n }

// firstSectionKeys reads one valid spec section-type key and one valid entity
// section-type key from the seeded vocabularies, so the Apply test can exercise
// both the "known key ⇒ insert" and "unknown key ⇒ skip" branches deterministically
// against whatever the migration seeded.
func firstSectionKeys(t *testing.T, ws *workspace.Workspace) (specKey, entityKey string) {
	t.Helper()
	ctx := context.Background()
	r, release, err := app.Reader(ctx, ws, "main")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	specTypes, err := store.ListSpecSectionTypes(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	entityTypes, err := store.ListEntitySectionTypes(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(specTypes) == 0 || len(entityTypes) == 0 {
		t.Fatalf("expected seeded section vocabularies, got spec=%d entity=%d", len(specTypes), len(entityTypes))
	}
	return specTypes[0].Key, entityTypes[0].Key
}

// buildGraph assembles a Graph that exercises every layer of Apply, including the
// "parent could not be resolved ⇒ skip and count" paths.
func buildGraph(specKey, entityKey string) *Graph {
	const badKey = "__definitely_not_a_seeded_section_key__"
	return &Graph{
		Domains: []Domain{
			{Slug: "enrollment", Name: "Enrollment", Description: "Enroll students"},
			{Slug: "scheduling", Name: "Scheduling"},
		},
		Milestones: []Milestone{
			{Slug: "M1"},      // real milestone
			{Slug: "tut-100"}, // beads issue id → NOT a milestone
		},
		Specs: []Spec{
			{
				Prefix: "ADDS", Path: "enrollment/add-student.md", Title: "Add Student",
				Domain: "enrollment", Status: "draft", Created: "2025-01-02",
				Sections:  []DocSection{{Key: specKey, Body: "known body"}, {Key: badKey, Body: "dropped"}},
				ReqGroups: []ReqGroup{{Position: 1, Title: "Core", Notes: "core notes"}},
			},
			// prefix-less doc: indexed by path only (exercises the sp.Prefix=="" branch).
			{Prefix: "", Path: "enrollment/notes.md", Title: "Notes", Domain: "enrollment", Status: "active"},
			// unresolved domain → spec skipped.
			{Prefix: "ORPH", Path: "ghost/x.md", Title: "Orphan", Domain: "nonexistent"},
		},
		Reqs: []Requirement{
			{FRKey: "ADDS-FR-001", SpecPrefix: "ADDS", Number: 1, Statement: "shall add a student",
				DeliveryStatus: "covered", Milestone: "M1", Section: "Core", Position: 1,
				Notes: "note", E2ERef: "e2e/add.spec.ts"},
			{FRKey: "ADDS-FR-002", SpecPrefix: "ADDS", Number: 2, Statement: "shall validate",
				DeliveryStatus: "not-implemented", Milestone: "tut-100", Position: 2}, // milestone is issue id → external ref
			{FRKey: "ADDS-FR-003", SpecPrefix: "ADDS", Number: 3, Statement: "obsolete rule",
				DeliveryStatus: "covered", Tombstoned: true}, // → content_status obsolete
			{FRKey: "ZZZ-FR-001", SpecPrefix: "ZZZ", Number: 1, Statement: "orphan"}, // unknown spec → skipped
		},
		Stories: []UserStory{
			{SpecPrefix: "ADDS", Position: 1, Title: "Enroll a student", Priority: 1,
				AsA: "registrar", IWant: "to add a student", SoThat: "they can attend"},
			{SpecPrefix: "ZZZ", Position: 1, Title: "orphan story"}, // unknown spec → skipped
		},
		Scenarios: []Scenario{
			{SpecPrefix: "ADDS", StoryPosition: 1, Position: 1, Given: "a roster", When: "I add", Then: "it appears"},
			{SpecPrefix: "ADDS", StoryPosition: 9, Position: 1, Given: "x", When: "y", Then: "z"}, // no such story → skipped
		},
		Entities: []Entity{
			{Name: "Student", Description: "A learner", Status: "active", DocPath: "entities/student.md",
				Sections: []DocSection{{Key: entityKey, Body: "purpose body"}, {Key: badKey, Body: "dropped"}}},
			{Name: "Course", Description: "A class", Status: "active", DocPath: "entities/course.md"},
		},
		Relationships: []EntityRelationship{
			{FromName: "Student", ToName: "Course", Cardinality: "many_to_many", JunctionTable: "enrollment"},
			{FromName: "Student", ToName: "Ghost", Cardinality: "one_to_many"}, // unknown endpoint → skipped
		},
		Refs: []EntityRef{
			{OwnerType: "requirement", OwnerKey: "ADDS-FR-001", TargetType: "spec", TargetKey: "enrollment/add-student.md"},
			{OwnerType: "spec", OwnerKey: "enrollment/add-student.md", TargetType: "entity", TargetKey: "Student"},
			{OwnerType: "domain", OwnerKey: "enrollment", TargetType: "milestone", TargetKey: "M1"},
			{OwnerType: "requirement", OwnerKey: "NOPE", TargetType: "spec", TargetKey: "enrollment/add-student.md"}, // owner unresolved → skipped
		},
		// planning layer
		Capabilities: []Capability{
			{SourceID: "C1", SourceURL: "https://notion/C1", Title: "Enrollment cap", Level: "capability",
				DomainSlug: "enrollment", MilestoneSlugs: []string{"M1"}, DeliverableSourceIDs: []string{"D1"}},
			{SourceID: "C2", Title: "bad domain cap", DomainSlug: "ghostdomain"}, // unresolved domain → skipped
		},
		Deliverables: []Deliverable{
			{SourceID: "D1", SourceURL: "https://notion/D1", Title: "Add student flow", Size: "M",
				Status: "built", AIReady: "yes", MilestoneSlug: "M1", ViewSourceIDs: []string{"V1"}, BeadIDs: "tut-500"},
			{SourceID: "D2", Title: "Blocked deliverable", BlockedBySourceIDs: []string{"D1"}},
		},
		Views: []View{
			{SourceID: "V1", SourceURL: "https://notion/V1", Title: "Add Student View", Route: "/add",
				DomainSlug: "enrollment", SpecFile: "add-student.md"}, // resolves to the ADDS spec by slug
			{SourceID: "V2", Title: "bad domain view", DomainSlug: "ghostdomain"}, // unresolved domain → skipped
		},
		// testing layer
		TestSuites: []TestSuite{
			{SourceID: "s1", Name: "Root Suite"},
			{SourceID: "s2", ParentSourceID: "s1", Name: "Child Suite", Position: pi(1)},
		},
		TestCases: []TestCase{
			{SourceID: "c1", SuiteSourceID: "s1", Title: "Add student case", Layer: "e2e",
				Steps: []TestStep{
					{Action: "open form", ExpectedResult: "form shown"},
					{Position: pi(5), Action: "submit", ExpectedResult: "saved"},
				},
				FRKeys: []string{"ADDS-FR-001", "NOPE-FR-9"}}, // one resolves, one does not
			{SourceID: "c2", SuiteSourceID: "", Title: "orphan case"}, // no suite → skipped
		},
		Configurations: []TestConfiguration{
			{SourceID: "cfg1", Group: "browser", Name: "chrome"},
		},
		TestRuns: []TestRun{
			{SourceID: "r1", Title: "Sprint 1", Status: "active", MilestoneSlug: "M1",
				ConfigSourceIDs: []string{"cfg1", "missing"}}, // one links, one is unknown
		},
		TestResults: []TestResult{
			{RunSourceID: "r1", CaseSourceID: "c1", ConfigSourceID: "cfg1", Status: "passed", DurationMs: pi(42)},
			{RunSourceID: "missing", CaseSourceID: "c1", Status: "passed"}, // unknown run → skipped
		},
	}
}

func TestApply_FullGraph(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	ctx := context.Background()
	specKey, entityKey := firstSectionKeys(t, ws)
	g := buildGraph(specKey, entityKey)

	// --- first apply: everything is inserted ---------------------------------
	var stats *ApplyStats
	if err := app.Mutate(ctx, ws, app.MutateOpts{Summary: "import", Changeset: "main", Actor: "importer-bot"},
		func(ctx context.Context, w *app.Write) error {
			s, e := Apply(ctx, w.Tx, g)
			stats = s
			for _, tbl := range TouchedTables {
				w.MarkDirty(tbl)
			}
			return e
		}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	eq := func(name string, got, want int) {
		t.Helper()
		if got != want {
			t.Errorf("%s = %d, want %d", name, got, want)
		}
	}
	// requirements layer
	eq("Inserted[domains]", stats.Inserted["domains"], 2)
	eq("Inserted[milestones]", stats.Inserted["milestones"], 1) // tut-100 is skipped, not a milestone
	eq("Inserted[specs]", stats.Inserted["specs"], 2)
	eq("Skipped[specs]", stats.Skipped["specs"], 1) // unresolved domain
	eq("Inserted[sections]", stats.Inserted["sections"], 2)
	eq("Skipped[sections]", stats.Skipped["sections"], 2) // one bad key per spec + entity
	eq("Inserted[requirement_groups]", stats.Inserted["requirement_groups"], 1)
	eq("Inserted[requirements]", stats.Inserted["requirements"], 3)
	eq("Skipped[requirements]", stats.Skipped["requirements"], 1) // unknown spec
	eq("Inserted[user_stories]", stats.Inserted["user_stories"], 1)
	eq("Skipped[user_stories]", stats.Skipped["user_stories"], 1)
	eq("Inserted[acceptance_scenarios]", stats.Inserted["acceptance_scenarios"], 1)
	eq("Skipped[acceptance_scenarios]", stats.Skipped["acceptance_scenarios"], 1)
	eq("Inserted[entities]", stats.Inserted["entities"], 2)
	eq("Inserted[relationships]", stats.Inserted["relationships"], 1)
	eq("Skipped[relationships]", stats.Skipped["relationships"], 1)
	eq("Inserted[entity_refs]", stats.Inserted["entity_refs"], 3)
	eq("Skipped[entity_refs]", stats.Skipped["entity_refs"], 1)
	// planning layer
	eq("Inserted[capabilities]", stats.Inserted["capabilities"], 1)
	eq("Skipped[capabilities]", stats.Skipped["capabilities"], 1)
	eq("Inserted[deliverables]", stats.Inserted["deliverables"], 2)
	eq("Inserted[views]", stats.Inserted["views"], 1)
	eq("Skipped[views]", stats.Skipped["views"], 1)
	eq("Inserted[capability_milestone]", stats.Inserted["capability_milestone"], 1)
	eq("Inserted[capability_deliverable]", stats.Inserted["capability_deliverable"], 1)
	eq("Inserted[deliverable_view]", stats.Inserted["deliverable_view"], 1)
	eq("Inserted[deliverable_dependency]", stats.Inserted["deliverable_dependency"], 1)
	// testing layer
	eq("Inserted[test_suites]", stats.Inserted["test_suites"], 2)
	eq("Inserted[test_cases]", stats.Inserted["test_cases"], 1)
	eq("Skipped[test_cases]", stats.Skipped["test_cases"], 1)
	eq("Inserted[test_steps]", stats.Inserted["test_steps"], 2)
	eq("Inserted[coverage]", stats.Inserted["coverage"], 1)
	eq("Skipped[coverage]", stats.Skipped["coverage"], 1)
	eq("Inserted[configurations]", stats.Inserted["configurations"], 1)
	eq("Inserted[test_runs]", stats.Inserted["test_runs"], 1)
	eq("Inserted[run_configs]", stats.Inserted["run_configs"], 1)
	eq("Inserted[test_results]", stats.Inserted["test_results"], 1)
	eq("Skipped[test_results]", stats.Skipped["test_results"], 1)

	// --- row-level assertions via a fresh reader (proves the commit persisted) --
	assertRows(t, ws, specKey)

	// --- second apply of the same graph: everything converges to updates ------
	var stats2 *ApplyStats
	if err := app.Mutate(ctx, ws, app.MutateOpts{Summary: "re-import", Changeset: "main", Actor: "importer-bot"},
		func(ctx context.Context, w *app.Write) error {
			s, e := Apply(ctx, w.Tx, g)
			stats2 = s
			for _, tbl := range TouchedTables {
				w.MarkDirty(tbl)
			}
			return e
		}); err != nil {
		t.Fatalf("re-Apply: %v", err)
	}
	if stats2.Inserted["domains"] != 0 {
		t.Errorf("re-apply Inserted[domains] = %d, want 0 (idempotent)", stats2.Inserted["domains"])
	}
	if stats2.Updated["domains"] != 2 {
		t.Errorf("re-apply Updated[domains] = %d, want 2", stats2.Updated["domains"])
	}
	if stats2.Updated["requirements"] != 3 {
		t.Errorf("re-apply Updated[requirements] = %d, want 3", stats2.Updated["requirements"])
	}
	if stats2.Updated["specs"] != 2 {
		t.Errorf("re-apply Updated[specs] = %d, want 2", stats2.Updated["specs"])
	}

	// A domain still has exactly one row after re-import (deterministic upsert, no dup).
	r, release, err := app.Reader(ctx, ws, "main")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	doms, err := store.ListDomains(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, d := range doms {
		if d.Slug == "enrollment" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("expected exactly 1 'enrollment' domain after re-import, got %d", n)
	}
}

// assertRows verifies persisted rows on main after the first apply. specKey is the
// seeded spec section key the graph used for its one valid section.
func assertRows(t *testing.T, ws *workspace.Workspace, specKey string) {
	t.Helper()
	ctx := context.Background()
	r, release, err := app.Reader(ctx, ws, "main")
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Milestones: M1 present, the beads issue id is not.
	ms, err := store.ListMilestones(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	seenMS := map[string]bool{}
	for _, m := range ms {
		seenMS[m.Slug] = true
	}
	if !seenMS["M1"] {
		t.Error("milestone M1 missing")
	}
	if seenMS["tut-100"] {
		t.Error("beads issue id tut-100 was stored as a milestone")
	}

	// Requirements of ADDS: three, with the milestone folded per issue-id rule.
	reqs, err := store.ListRequirements(ctx, r, "ADDS")
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 3 {
		t.Fatalf("ADDS requirements = %d, want 3", len(reqs))
	}
	byKey := map[string]store.Requirement{}
	for _, q := range reqs {
		byKey[q.FRKey] = q
	}
	if byKey["ADDS-FR-001"].MilestoneID == "" {
		t.Error("ADDS-FR-001 should have milestone M1 resolved")
	}
	if byKey["ADDS-FR-002"].MilestoneID != "" {
		t.Error("ADDS-FR-002 milestone is a beads issue id and must NOT resolve to a milestone row")
	}

	// e2e_ref folded into notes; tombstone → content_status obsolete.
	fr1, ok, err := store.GetRequirement(ctx, r, "ADDS-FR-001")
	if err != nil || !ok {
		t.Fatalf("GetRequirement ADDS-FR-001: ok=%v err=%v", ok, err)
	}
	if fr1.Notes != "note [e2e: e2e/add.spec.ts]" {
		t.Errorf("ADDS-FR-001 notes = %q, want e2e folded", fr1.Notes)
	}
	fr3, _, err := store.GetRequirement(ctx, r, "ADDS-FR-003")
	if err != nil {
		t.Fatal(err)
	}
	if fr3.ContentStatus != "obsolete" {
		t.Errorf("ADDS-FR-003 content_status = %q, want obsolete", fr3.ContentStatus)
	}

	// The issue-id milestone became an external ref on ADDS-FR-002.
	fr2, _, err := store.GetRequirement(ctx, r, "ADDS-FR-002")
	if err != nil {
		t.Fatal(err)
	}
	refs, err := store.ListExternalRefs(ctx, r, "requirement", fr2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].System != "beads" || refs[0].ExternalID != "tut-100" {
		t.Fatalf("ADDS-FR-002 external refs = %+v, want one beads/tut-100", refs)
	}

	// Specs: ADDS + the prefix-less doc; the orphan (bad domain) is absent.
	specs, err := store.ListSpecs(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	specSlugs := map[string]bool{}
	for _, s := range specs {
		specSlugs[s.Slug] = true
	}
	if !specSlugs["add-student"] || !specSlugs["notes"] {
		t.Errorf("expected add-student and notes specs, got %v", specSlugs)
	}
	if specSlugs["x"] {
		t.Error("orphan spec (unresolved domain) should not have been written")
	}

	// Spec sections: the known key landed, the unknown key was dropped.
	var addsID string
	for _, s := range specs {
		if s.Prefix == "ADDS" {
			addsID = s.ID
		}
	}
	secs, err := store.ListSpecSections(ctx, r, addsID)
	if err != nil {
		t.Fatal(err)
	}
	if len(secs) != 1 || secs[0].Key != specKey {
		t.Fatalf("ADDS sections = %+v, want exactly the one seeded key %q", secs, specKey)
	}

	// Entities and their relationship persisted.
	ents, err := store.ListEntities(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	entByName := map[string]store.EntityRow{}
	for _, e := range ents {
		entByName[e.Name] = e
	}
	if _, ok := entByName["Student"]; !ok {
		t.Error("entity Student missing")
	}
	if _, ok := entByName["Course"]; !ok {
		t.Error("entity Course missing")
	}
	rels, err := store.ListEntityRelationships(ctx, r, entByName["Student"].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 1 || rels[0].OtherName != "Course" {
		t.Fatalf("Student relationships = %+v, want one to Course", rels)
	}

	// Planning rows and a couple of junctions.
	caps, err := store.ListCapabilities(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(caps) != 1 || caps[0].Title != "Enrollment cap" {
		t.Fatalf("capabilities = %+v, want one (bad-domain one skipped)", caps)
	}
	delivs, err := store.ListDeliverables(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(delivs) != 2 {
		t.Fatalf("deliverables = %d, want 2", len(delivs))
	}
	pviews, err := store.ListPlanViews(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(pviews) != 1 {
		t.Fatalf("plan views = %d, want 1", len(pviews))
	}
	if pviews[0].SpecTitle != "Add Student" {
		t.Errorf("view spec title = %q, want the resolved ADDS spec title", pviews[0].SpecTitle)
	}
	if pairs, err := store.ListCapabilityDeliverablePairs(ctx, r); err != nil {
		t.Fatal(err)
	} else if len(pairs) != 1 {
		t.Errorf("capability_deliverable pairs = %d, want 1", len(pairs))
	}
	if pairs, err := store.ListDeliverableDependencyPairs(ctx, r); err != nil {
		t.Fatal(err)
	} else if len(pairs) != 1 {
		t.Errorf("deliverable_dependency pairs = %d, want 1", len(pairs))
	}

	// Planning external refs (Notion source links + the deliverable's Bead id).
	pref, err := store.ListExternalRefsForSubjects(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	var notion, beads int
	for _, x := range pref {
		switch x.System {
		case "notion":
			notion++
		case "beads":
			beads++
		}
	}
	if notion < 3 { // capability + deliverable + view
		t.Errorf("planning notion external refs = %d, want >= 3", notion)
	}
	if beads < 1 { // D1 BeadIDs
		t.Errorf("planning beads external refs = %d, want >= 1", beads)
	}

	// Testing rows.
	suites, err := store.ListTestSuites(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(suites) != 2 {
		t.Fatalf("test suites = %d, want 2", len(suites))
	}
	var childParent string
	for _, s := range suites {
		if s.Name == "Child Suite" {
			childParent = s.ParentID
		}
	}
	if childParent == "" {
		t.Error("child suite parent_id not set in the second pass")
	}
	cases, err := store.ListTestCases(ctx, r, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 || cases[0].Title != "Add student case" {
		t.Fatalf("test cases = %+v, want the one with a resolved suite", cases)
	}
	steps, err := store.ListTestSteps(ctx, r, cases[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 {
		t.Fatalf("test steps = %d, want 2", len(steps))
	}
	runs, err := store.ListTestRuns(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("test runs = %d, want 1", len(runs))
	}
	if runs[0].MilestoneSlug != "M1" {
		t.Errorf("run milestone = %q, want M1", runs[0].MilestoneSlug)
	}
	results, err := store.ListTestResults(ctx, r, runs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != "passed" {
		t.Fatalf("test results = %+v, want one passed", results)
	}
}

// TestApply_MinimalGraph exercises the early-return branches of the planning and
// testing sub-appliers (no planning/testing rows to write) and a single domain.
func TestApply_MinimalGraph(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	ctx := context.Background()
	g := &Graph{Domains: []Domain{{Slug: "solo", Name: "Solo"}}}

	var stats *ApplyStats
	if err := app.Mutate(ctx, ws, app.MutateOpts{Summary: "minimal", Changeset: "main", Actor: "importer-bot"},
		func(ctx context.Context, w *app.Write) error {
			s, e := Apply(ctx, w.Tx, g)
			stats = s
			w.MarkDirty("req_domain")
			return e
		}); err != nil {
		t.Fatalf("Apply minimal: %v", err)
	}
	if stats.Inserted["domains"] != 1 {
		t.Errorf("Inserted[domains] = %d, want 1", stats.Inserted["domains"])
	}
	// No planning/testing rows were staged, so those counters stay zero.
	if stats.Inserted["capabilities"] != 0 || stats.Inserted["test_suites"] != 0 {
		t.Errorf("minimal graph wrote planning/testing rows: %+v", stats.Inserted)
	}

	r, release, err := app.Reader(ctx, ws, "main")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	doms, err := store.ListDomains(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range doms {
		if d.Slug == "solo" {
			found = true
		}
	}
	if !found {
		t.Error("solo domain not persisted")
	}
}
