package store_test

import (
	"context"
	"testing"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/enums"
	"github.com/endermalkoc/cusp/internal/store"
	"github.com/endermalkoc/cusp/internal/testutil"
	"github.com/endermalkoc/cusp/internal/workspace"
)

// allTables is every table any store write in this test touches. The mutation
// wrapper stages only tables marked dirty, so a body must mark everything it wrote
// or the next command's clean-working-set preflight fails. Marking an untouched
// table is a harmless DOLT_ADD no-op, so the write helper marks them all.
var allTables = []string{
	"req_domain", "req_spec", "req_requirement", "req_requirement_group",
	"req_user_story", "req_acceptance_scenario", "req_glossary_term", "req_glossary_alias",
	"req_spec_section", "req_spec_section_type", "req_entity_ref", "req_edge",
	"req_requirement_test_case",
	"ent_entity", "ent_entity_section", "ent_entity_section_type", "ent_relationship",
	"pub_external_ref",
	"plan_milestone", "plan_capability", "plan_deliverable", "plan_view",
	"plan_capability_milestone", "plan_capability_deliverable",
	"plan_deliverable_view", "plan_deliverable_dependency",
	"test_suite", "test_case", "test_step", "test_configuration", "test_run",
	"test_result", "test_run_configuration",
	"rev_actor", "rev_changeset", "rev_comment", "rev_review",
}

func iptr(i int) *int { return &i }

// write runs body as one committed mutation on main, marking every table dirty.
func write(t *testing.T, ws *workspace.Workspace, body func(ctx context.Context, tx store.Execer) error) {
	t.Helper()
	err := app.Mutate(context.Background(), ws,
		app.MutateOpts{Summary: "test write", Changeset: "main", Actor: "tester"},
		func(ctx context.Context, w *app.Write) error {
			for _, tbl := range allTables {
				w.MarkDirty(tbl)
			}
			return body(ctx, w.Tx)
		})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
}

// read opens a branch-pinned reader on main and runs fn against it.
func read(t *testing.T, ws *workspace.Workspace, fn func(ctx context.Context, r store.Execer)) {
	t.Helper()
	ctx := context.Background()
	r, release, err := app.Reader(ctx, ws, "main")
	if err != nil {
		t.Fatalf("reader: %v", err)
	}
	defer release()
	fn(ctx, r)
}

// TestStoreRoundTrips drives one insert → read → update → read → delete → read pass
// across every store family against a real Dolt server.
func TestStoreRoundTrips(t *testing.T) {
	ws := testutil.NewWorkspace(t)

	// ids captured across phases.
	var (
		schedID, attSpecID            string
		attReqID                      string
		usID                          string
		eStudentID, eCourseID         string
		glossID                       string
		milestone1ID                  string
		suiteID, caseID, cfgID, runID string
		step1ID                       string
		authorID, csID                string
		topCommentID, replyCommentID  string
		beadRefID                     string
	)
	const (
		capID  = "cap-1"
		del1ID = "del-1"
		del2ID = "del-2"
		viewID = "view-1"
	)

	// ---------------------------------------------------------------- inserts
	write(t, ws, func(ctx context.Context, tx store.Execer) error {
		// domains
		d, err := store.AddDomain(ctx, tx, store.Domain{Slug: "scheduling", Name: "Scheduling", Description: "sched"})
		if err != nil {
			return err
		}
		if d.ID == "" || d.Status != "active" {
			t.Errorf("AddDomain returned %+v", d)
		}
		schedID = d.ID
		if id, ins, err := store.UpsertDomain(ctx, tx, store.Domain{Slug: "planning", Name: "Planning"}); err != nil {
			return err
		} else if !ins || id == "" {
			t.Errorf("UpsertDomain first: ins=%v id=%q, want insert", ins, id)
		}
		if _, ins, err := store.UpsertDomain(ctx, tx, store.Domain{Slug: "planning", Name: "Planning v2"}); err != nil {
			return err
		} else if ins {
			t.Errorf("UpsertDomain second: want update, got insert")
		}

		// specs
		sp, err := store.AddSpec(ctx, tx, "scheduling", store.Spec{Prefix: "ATT", Slug: "take-attendance", Path: "events", Title: "Take Attendance"})
		if err != nil {
			return err
		}
		if sp.ID == "" || sp.Status != "active" {
			t.Errorf("AddSpec returned %+v", sp)
		}
		attSpecID = sp.ID
		if id, ins, err := store.UpsertSpec(ctx, tx, schedID, store.Spec{Prefix: "SCH", Slug: "schedule", Title: "Scheduling", CreatedAt: "2024-01-15"}); err != nil {
			return err
		} else if !ins || id == "" {
			t.Errorf("UpsertSpec by prefix first: ins=%v", ins)
		}
		if _, ins, err := store.UpsertSpec(ctx, tx, schedID, store.Spec{Prefix: "SCH", Slug: "schedule", Title: "Scheduling v2"}); err != nil {
			return err
		} else if ins {
			t.Errorf("UpsertSpec by prefix second: want update")
		}
		if _, ins, err := store.UpsertSpec(ctx, tx, schedID, store.Spec{Slug: "top-level", Title: "Top"}); err != nil {
			return err
		} else if !ins {
			t.Errorf("UpsertSpec prefix-less: want insert")
		}

		// requirements (fr_key derivation)
		r1, err := store.AddRequirement(ctx, tx, "ATT", store.Requirement{Statement: "The system shall record attendance."})
		if err != nil {
			return err
		}
		if r1.Number != 1 || r1.FRKey != "ATT-FR-001" || r1.ContentStatus != "active" {
			t.Errorf("AddRequirement #1: %+v", r1)
		}
		attReqID = r1.ID
		r2, err := store.AddRequirement(ctx, tx, "ATT", store.Requirement{Statement: "second", Suffix: "a", Priority: iptr(2)})
		if err != nil {
			return err
		}
		if r2.Number != 2 || r2.FRKey != "ATT-FR-002a" {
			t.Errorf("AddRequirement #2: %+v", r2)
		}
		if id, ins, err := store.UpsertRequirement(ctx, tx, attSpecID, store.Requirement{Number: 5, FRKey: "ATT-FR-005", Statement: "imported", Position: 1}); err != nil {
			return err
		} else if !ins || id == "" {
			t.Errorf("UpsertRequirement first: ins=%v", ins)
		}
		if _, ins, err := store.UpsertRequirement(ctx, tx, attSpecID, store.Requirement{Number: 5, FRKey: "ATT-FR-005", Statement: "imported v2"}); err != nil {
			return err
		} else if ins {
			t.Errorf("UpsertRequirement second: want update")
		}

		// requirement group
		if gid, ins, err := store.UpsertRequirementGroup(ctx, tx, attSpecID, 1, "Core", "grp notes"); err != nil {
			return err
		} else if !ins || gid == "" {
			t.Errorf("UpsertRequirementGroup first: ins=%v", ins)
		}
		if _, ins, err := store.UpsertRequirementGroup(ctx, tx, attSpecID, 2, "Core", "grp notes2"); err != nil {
			return err
		} else if ins {
			t.Errorf("UpsertRequirementGroup second: want update")
		}

		// user story + scenario
		id, ins, err := store.UpsertUserStory(ctx, tx, store.UserStory{
			SpecID: attSpecID, Position: 1, Title: "Story", Priority: 1,
			AsA: "teacher", IWant: "to record", SoThat: "tracking",
			Narrative: "narr", WhyPriority: "why", IndependentTest: "indep",
		})
		if err != nil {
			return err
		}
		if !ins || id == "" {
			t.Errorf("UpsertUserStory first: ins=%v", ins)
		}
		usID = id
		if _, ins, err := store.UpsertUserStory(ctx, tx, store.UserStory{SpecID: attSpecID, Position: 1, Title: "Story v2", Priority: 2}); err != nil {
			return err
		} else if ins {
			t.Errorf("UpsertUserStory second: want update")
		}
		if _, ins, err := store.UpsertScenario(ctx, tx, store.Scenario{UserStoryID: usID, Position: 1, Given: "g", When: "w", Then: "t"}); err != nil {
			return err
		} else if !ins {
			t.Errorf("UpsertScenario first: want insert")
		}
		if _, ins, err := store.UpsertScenario(ctx, tx, store.Scenario{UserStoryID: usID, Position: 1, Given: "g2"}); err != nil {
			return err
		} else if ins {
			t.Errorf("UpsertScenario second: want update")
		}

		// entities + sections + relationship
		if eStudentID, ins, err = store.UpsertEntity(ctx, tx, store.Entity{Name: "Student", Description: "a student", Status: "active"}); err != nil {
			return err
		} else if !ins {
			t.Errorf("UpsertEntity Student: want insert")
		}
		if _, ins, err := store.UpsertEntity(ctx, tx, store.Entity{Name: "Student", Description: "a student v2"}); err != nil {
			return err
		} else if ins {
			t.Errorf("UpsertEntity Student second: want update")
		}
		if eCourseID, _, err = store.UpsertEntity(ctx, tx, store.Entity{Name: "Course", Description: "a course"}); err != nil {
			return err
		}
		if _, ins, err := store.UpsertEntitySection(ctx, tx, eStudentID, "purpose", "why students"); err != nil {
			return err
		} else if !ins {
			t.Errorf("UpsertEntitySection first: want insert")
		}
		if _, ins, err := store.UpsertEntitySection(ctx, tx, eStudentID, "purpose", "why students v2"); err != nil {
			return err
		} else if ins {
			t.Errorf("UpsertEntitySection second: want update")
		}
		if _, ins, err := store.UpsertSpecSection(ctx, tx, attSpecID, "overview", "the overview"); err != nil {
			return err
		} else if !ins {
			t.Errorf("UpsertSpecSection first: want insert")
		}
		if _, ins, err := store.UpsertSpecSection(ctx, tx, attSpecID, "overview", "the overview v2"); err != nil {
			return err
		} else if ins {
			t.Errorf("UpsertSpecSection second: want update")
		}
		if _, err := store.UpsertEntityRelationship(ctx, tx, eStudentID, eCourseID, "1:N", "enrollment"); err != nil {
			return err
		}

		// authored section types
		if ins, err := store.UpsertSpecSectionType(ctx, tx, store.SectionTypeRow{Key: "custom_spec", Title: "Custom", Level: 2, Position: 100, Description: "d", Origin: "authored"}); err != nil {
			return err
		} else if !ins {
			t.Errorf("UpsertSpecSectionType first: want insert")
		}
		if ins, err := store.UpsertSpecSectionType(ctx, tx, store.SectionTypeRow{Key: "custom_spec", Title: "Custom v2", Level: 3, Position: 101}); err != nil {
			return err
		} else if ins {
			t.Errorf("UpsertSpecSectionType second: want update")
		}
		if ins, err := store.UpsertEntitySectionType(ctx, tx, store.SectionTypeRow{Key: "custom_ent", Title: "CustomE", Level: 2, Position: 100}); err != nil {
			return err
		} else if !ins {
			t.Errorf("UpsertEntitySectionType first: want insert")
		}
		if _, err := store.UpsertEntitySectionType(ctx, tx, store.SectionTypeRow{Key: "custom_ent", Title: "CustomE v2"}); err != nil {
			return err
		}

		// glossary
		if glossID, ins, err = store.UpsertGlossaryTerm(ctx, tx, store.GlossaryTerm{Slug: "attendance", Term: "Attendance", Definition: "being present", DomainID: schedID, Status: "active"}); err != nil {
			return err
		} else if !ins {
			t.Errorf("UpsertGlossaryTerm first: want insert")
		}
		if _, ins, err := store.UpsertGlossaryTerm(ctx, tx, store.GlossaryTerm{Slug: "attendance", Term: "Attendance v2"}); err != nil {
			return err
		} else if ins {
			t.Errorf("UpsertGlossaryTerm second: want update")
		}
		if err := store.SetGlossaryAliases(ctx, tx, glossID, []string{"presence", "attend", "presence", ""}); err != nil {
			return err
		}

		// edges + entity refs
		e1, err := store.AddEdgeByIDs(ctx, tx, "requirement", attReqID, "depends_on", "spec", attSpecID)
		if err != nil {
			return err
		}
		e2, err := store.AddEdgeByIDs(ctx, tx, "requirement", attReqID, "depends_on", "spec", attSpecID)
		if err != nil {
			return err
		}
		if e1 != e2 {
			t.Errorf("AddEdgeByIDs not deterministic: %q vs %q", e1, e2)
		}
		if _, err := store.UpsertEntityRef(ctx, tx, "spec", attSpecID, "entity", eStudentID); err != nil {
			return err
		}
		if _, err := store.UpsertEntityRef(ctx, tx, "spec", attSpecID, "entity", eStudentID); err != nil {
			return err
		}

		// external ref on requirement
		if beadRefID, ins, err = store.UpsertExternalRef(ctx, tx, "requirement", attReqID, "beads", "bd-1", "http://x"); err != nil {
			return err
		} else if !ins {
			t.Errorf("UpsertExternalRef first: want insert")
		}
		if _, ins, err := store.UpsertExternalRef(ctx, tx, "requirement", attReqID, "beads", "bd-1b", "http://x2"); err != nil {
			return err
		} else if ins {
			t.Errorf("UpsertExternalRef second: want update")
		}

		// planning
		if milestone1ID, ins, err = store.UpsertMilestone(ctx, tx, store.Milestone{Slug: "m1", Name: "M1"}); err != nil {
			return err
		} else if !ins {
			t.Errorf("UpsertMilestone first: want insert")
		}
		if _, ins, err := store.UpsertMilestone(ctx, tx, store.Milestone{Slug: "m1", Name: "M1 again"}); err != nil {
			return err
		} else if ins {
			t.Errorf("UpsertMilestone second: want update")
		}
		if m2, err := store.AddMilestone(ctx, tx, store.MilestoneRow{Slug: "m2", Name: "M2", Sequence: iptr(2)}); err != nil {
			return err
		} else if m2.ID == "" || m2.Status != "pending" {
			t.Errorf("AddMilestone returned %+v", m2)
		}
		if ins, err := store.UpsertCapability(ctx, tx, store.Capability{ID: capID, Title: "Cap", Level: "epic", DomainID: schedID}); err != nil {
			return err
		} else if !ins {
			t.Errorf("UpsertCapability first: want insert")
		}
		if ins, err := store.UpsertCapability(ctx, tx, store.Capability{ID: capID, Title: "Cap v2", DomainID: schedID}); err != nil {
			return err
		} else if ins {
			t.Errorf("UpsertCapability second: want update")
		}
		if err := store.SetCapabilityParent(ctx, tx, capID, ""); err != nil {
			return err
		}
		if ins, err := store.UpsertDeliverable(ctx, tx, store.Deliverable{ID: del1ID, Title: "Del", Size: "M", AIReady: "yes", MilestoneID: milestone1ID}); err != nil {
			return err
		} else if !ins {
			t.Errorf("UpsertDeliverable first: want insert")
		}
		if ins, err := store.UpsertDeliverable(ctx, tx, store.Deliverable{ID: del1ID, Title: "Del v2", Status: "built"}); err != nil {
			return err
		} else if ins {
			t.Errorf("UpsertDeliverable second: want update")
		}
		if _, err := store.UpsertDeliverable(ctx, tx, store.Deliverable{ID: del2ID, Title: "Del2"}); err != nil {
			return err
		}
		if ins, err := store.UpsertView(ctx, tx, store.View{ID: viewID, Title: "View", Route: "/v", SpecID: attSpecID, DomainID: schedID}); err != nil {
			return err
		} else if !ins {
			t.Errorf("UpsertView first: want insert")
		}
		if ins, err := store.UpsertView(ctx, tx, store.View{ID: viewID, Title: "View v2", DomainID: schedID}); err != nil {
			return err
		} else if ins {
			t.Errorf("UpsertView second: want update")
		}
		for _, link := range []func() error{
			func() error { return store.LinkCapabilityMilestone(ctx, tx, capID, milestone1ID) },
			func() error { return store.LinkCapabilityDeliverable(ctx, tx, capID, del1ID) },
			func() error { return store.LinkDeliverableView(ctx, tx, del1ID, viewID) },
			func() error { return store.LinkDeliverableDependency(ctx, tx, del1ID, del2ID) },
		} {
			if err := link(); err != nil {
				return err
			}
		}
		if _, _, err := store.UpsertExternalRef(ctx, tx, "deliverable", del1ID, "notion", "page-1", "http://n"); err != nil {
			return err
		}

		// testing
		suite, err := store.AddTestSuite(ctx, tx, store.TestSuiteRow{Name: "Suite A", Description: "d", Position: iptr(1)})
		if err != nil {
			return err
		}
		suiteID = suite.ID
		if _, err := store.AddTestSuite(ctx, tx, store.TestSuiteRow{ParentID: suiteID, Name: "Child"}); err != nil {
			return err
		}
		tc, err := store.AddTestCase(ctx, tx, store.TestCaseRow{SuiteID: suiteID, Title: "Case", Priority: iptr(2), Layer: "unit", Type: "functional"})
		if err != nil {
			return err
		}
		if tc.Status != "draft" {
			t.Errorf("AddTestCase default status = %q, want draft", tc.Status)
		}
		caseID = tc.ID
		s1, err := store.AddTestStep(ctx, tx, store.TestStepRow{TestCaseID: caseID, Action: "do", ExpectedResult: "ok"})
		if err != nil {
			return err
		}
		if s1.Position == nil || *s1.Position != 1 {
			t.Errorf("AddTestStep first position = %v, want 1", s1.Position)
		}
		step1ID = s1.ID
		if s2, err := store.AddTestStep(ctx, tx, store.TestStepRow{TestCaseID: caseID, Action: "next"}); err != nil {
			return err
		} else if s2.Position == nil || *s2.Position != 2 {
			t.Errorf("AddTestStep second position = %v, want 2", s2.Position)
		}
		cfg, err := store.AddConfiguration(ctx, tx, store.ConfigurationRow{Group: "browser", Name: "chrome", Description: "d"})
		if err != nil {
			return err
		}
		cfgID = cfg.ID
		run, err := store.AddTestRun(ctx, tx, store.TestRunRow{Title: "Run 1"}, milestone1ID)
		if err != nil {
			return err
		}
		if run.Status != "active" {
			t.Errorf("AddTestRun default status = %q, want active", run.Status)
		}
		runID = run.ID
		if err := store.LinkRequirementTestCase(ctx, tx, attReqID, caseID); err != nil {
			return err
		}
		if err := store.LinkRunConfiguration(ctx, tx, runID, cfgID); err != nil {
			return err
		}
		res1, err := store.UpsertTestResult(ctx, tx, store.TestResultRow{RunID: runID, TestCaseID: caseID, ConfigurationID: cfgID, Status: "passed", Comment: "c", DurationMs: iptr(100), ExecutedBy: "bot"})
		if err != nil {
			return err
		}
		res2, err := store.UpsertTestResult(ctx, tx, store.TestResultRow{RunID: runID, TestCaseID: caseID, ConfigurationID: cfgID, Status: "failed"})
		if err != nil {
			return err
		}
		if res1.ID != res2.ID {
			t.Errorf("UpsertTestResult id not deterministic: %q vs %q", res1.ID, res2.ID)
		}
		// import-style upserts (caller-supplied ids)
		for _, up := range []struct {
			name string
			fn   func() (bool, error)
		}{
			{"suite", func() (bool, error) {
				return store.UpsertTestSuite(ctx, tx, store.TestSuiteRow{ID: "ts-imp", Name: "US"})
			}},
			{"case", func() (bool, error) {
				return store.UpsertTestCase(ctx, tx, store.TestCaseRow{ID: "tc-imp", SuiteID: suiteID, Title: "UC"})
			}},
			{"step", func() (bool, error) {
				return store.UpsertTestStep(ctx, tx, store.TestStepRow{ID: "tsp-imp", TestCaseID: "tc-imp", Action: "a"})
			}},
			{"config", func() (bool, error) {
				return store.UpsertConfiguration(ctx, tx, store.ConfigurationRow{ID: "cfg-imp", Group: "g", Name: "n"})
			}},
			{"run", func() (bool, error) {
				return store.UpsertTestRun(ctx, tx, store.TestRunRow{ID: "tr-imp", Title: "UR"}, "")
			}},
		} {
			ins, err := up.fn()
			if err != nil {
				return err
			}
			if !ins {
				t.Errorf("Upsert %s first: want insert", up.name)
			}
			ins, err = up.fn()
			if err != nil {
				return err
			}
			if ins {
				t.Errorf("Upsert %s second: want update", up.name)
			}
		}
		if err := store.SetTestSuiteParent(ctx, tx, "ts-imp", ""); err != nil {
			return err
		}

		// review layer: actor + changeset + comments + review
		if authorID, err = store.SeedActor(ctx, tx, "reviewer", "Reviewer"); err != nil {
			return err
		}
		if again, err := store.SeedActor(ctx, tx, "reviewer", "Reviewer"); err != nil {
			return err
		} else if again != authorID {
			t.Errorf("SeedActor not idempotent: %q vs %q", authorID, again)
		}
		csID = "cs-test-1"
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO `rev_changeset` (id,title,author_id,status,branch) VALUES (?,?,?,?,?)",
			csID, "Test CS", authorID, enums.ChangesetDraft, "changeset/test"); err != nil {
			return err
		}
		top, err := store.AddComment(ctx, tx, csID, authorID, "", "top comment", "requirement", attReqID, "field")
		if err != nil {
			return err
		}
		topCommentID = top.ID
		reply, err := store.AddComment(ctx, tx, csID, authorID, topCommentID, "reply", "", "", "")
		if err != nil {
			return err
		}
		replyCommentID = reply.ID
		if rv, err := store.UpsertReview(ctx, tx, csID, authorID, "approve", "lgtm"); err != nil {
			return err
		} else if rv.Verdict != "approve" {
			t.Errorf("UpsertReview verdict = %q", rv.Verdict)
		}
		return nil
	})

	// ---------------------------------------------------------- read (insert)
	read(t, ws, func(ctx context.Context, r store.Execer) {
		ds, err := store.ListDomains(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		if !containsFunc(ds, func(d store.Domain) bool { return d.Slug == "scheduling" }) ||
			!containsFunc(ds, func(d store.Domain) bool { return d.Slug == "planning" }) {
			t.Errorf("ListDomains missing expected slugs: %+v", ds)
		}
		if d, ok, err := store.GetDomain(ctx, r, "scheduling"); err != nil || !ok || d.Name != "Scheduling" {
			t.Errorf("GetDomain scheduling: %+v ok=%v err=%v", d, ok, err)
		}
		if _, ok, err := store.GetDomain(ctx, r, "nope"); err != nil || ok {
			t.Errorf("GetDomain nope: ok=%v err=%v", ok, err)
		}
		if id, ok, err := store.DomainIDBySlug(ctx, r, "scheduling"); err != nil || !ok || id != schedID {
			t.Errorf("DomainIDBySlug: %q ok=%v err=%v", id, ok, err)
		}
		if _, ok, _ := store.DomainIDBySlug(ctx, r, "nope"); ok {
			t.Error("DomainIDBySlug nope: want !ok")
		}

		specs, err := store.ListSpecs(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		var sch store.SpecRow
		for _, s := range specs {
			if s.Prefix == "SCH" {
				sch = s
			}
		}
		if sch.Created != "2024-01-15" {
			t.Errorf("SCH created = %q, want 2024-01-15 (%+v)", sch.Created, specs)
		}
		for _, key := range []string{"ATT", "scheduling/events/take-attendance.md", "scheduling/events/take-attendance"} {
			if s, ok, err := store.GetSpec(ctx, r, key); err != nil || !ok || s.ID != attSpecID {
				t.Errorf("GetSpec %q: %+v ok=%v err=%v", key, s, ok, err)
			}
		}
		if _, ok, _ := store.GetSpec(ctx, r, "nope"); ok {
			t.Error("GetSpec nope: want !ok")
		}
		if id, ok, err := store.SpecIDByPrefix(ctx, r, "ATT"); err != nil || !ok || id != attSpecID {
			t.Errorf("SpecIDByPrefix ATT: %q ok=%v", id, ok)
		}
		if _, ok, _ := store.SpecIDByPrefix(ctx, r, "NOPE"); ok {
			t.Error("SpecIDByPrefix NOPE: want !ok")
		}
		if id, ok, err := store.FindSpecIDBySlug(ctx, r, "take-attendance"); err != nil || !ok || id != attSpecID {
			t.Errorf("FindSpecIDBySlug: %q ok=%v", id, ok)
		}
		if _, ok, _ := store.FindSpecIDBySlug(ctx, r, "no-such-slug"); ok {
			t.Error("FindSpecIDBySlug missing: want !ok")
		}

		reqs, err := store.ListRequirements(ctx, r, "ATT")
		if err != nil {
			t.Fatal(err)
		}
		if len(reqs) != 3 || reqs[0].FRKey != "ATT-FR-001" || reqs[1].FRKey != "ATT-FR-002a" {
			t.Errorf("ListRequirements = %+v", reqs)
		}
		if rq, ok, err := store.GetRequirement(ctx, r, "ATT-FR-001"); err != nil || !ok || rq.Priority != nil {
			t.Errorf("GetRequirement 001: %+v ok=%v", rq, ok)
		}
		if rq, ok, err := store.GetRequirement(ctx, r, "ATT-FR-002a"); err != nil || !ok || rq.Priority == nil || *rq.Priority != 2 {
			t.Errorf("GetRequirement 002a priority: %+v ok=%v", rq, ok)
		}
		if _, ok, _ := store.GetRequirement(ctx, r, "NOPE-FR-001"); ok {
			t.Error("GetRequirement missing: want !ok")
		}
		if rows, err := store.ListReqsBySpecID(ctx, r, attSpecID); err != nil || len(rows) != 3 {
			t.Errorf("ListReqsBySpecID = %d rows err=%v", len(rows), err)
		}
		if groups, err := store.ListReqGroups(ctx, r, attSpecID); err != nil || len(groups) != 1 || groups[0].Title != "Core" {
			t.Errorf("ListReqGroups = %+v err=%v", groups, err)
		}

		// The second UpsertUserStory/UpsertScenario updated the same rows with sparse
		// data, so the persisted state reflects that second write.
		if stories, err := store.ListStoriesBySpec(ctx, r, attSpecID); err != nil || len(stories) != 1 || stories[0].Title != "Story v2" || stories[0].Priority != 2 {
			t.Errorf("ListStoriesBySpec = %+v err=%v", stories, err)
		}
		if scen, err := store.ListScenariosByStory(ctx, r, usID); err != nil || len(scen) != 1 || scen[0].Given != "g2" {
			t.Errorf("ListScenariosByStory = %+v err=%v", scen, err)
		}

		if pr, err := store.ListPriorities(ctx, r); err != nil {
			t.Fatal(err)
		} else if len(pr) > 0 {
			if _, ok, err := store.PriorityByLevel(ctx, r, pr[0].Level); err != nil || !ok {
				t.Errorf("PriorityByLevel(%d): ok=%v err=%v", pr[0].Level, ok, err)
			}
		}
		if _, ok, _ := store.PriorityByLevel(ctx, r, 999); ok {
			t.Error("PriorityByLevel(999): want !ok")
		}
		if ds, err := store.ListDeliveryStatuses(ctx, r); err != nil {
			t.Fatal(err)
		} else if len(ds) > 0 {
			if _, ok, err := store.DeliveryStatusByKey(ctx, r, ds[0].Key); err != nil || !ok {
				t.Errorf("DeliveryStatusByKey(%q): ok=%v", ds[0].Key, ok)
			}
		}
		if _, ok, _ := store.DeliveryStatusByKey(ctx, r, "no-such-status"); ok {
			t.Error("DeliveryStatusByKey missing: want !ok")
		}

		ents, err := store.ListEntities(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		var student store.EntityRow
		for _, e := range ents {
			if e.Name == "Student" {
				student = e
			}
		}
		if student.DocPath != "entities/student.md" {
			t.Errorf("Student DocPath = %q", student.DocPath)
		}
		if e, ok, err := store.GetEntity(ctx, r, "Student"); err != nil || !ok || e.ID != eStudentID {
			t.Errorf("GetEntity Student: %+v ok=%v", e, ok)
		}
		if _, ok, _ := store.GetEntity(ctx, r, "Nobody"); ok {
			t.Error("GetEntity missing: want !ok")
		}
		if id, ok, err := store.EntityIDByName(ctx, r, "Student"); err != nil || !ok || id != eStudentID {
			t.Errorf("EntityIDByName: %q ok=%v", id, ok)
		}
		if _, ok, _ := store.EntityIDByName(ctx, r, "Nobody"); ok {
			t.Error("EntityIDByName missing: want !ok")
		}
		if secs, err := store.ListEntitySections(ctx, r, eStudentID); err != nil || len(secs) != 1 || secs[0].Key != "purpose" {
			t.Errorf("ListEntitySections = %+v err=%v", secs, err)
		}
		if secs, err := store.ListSpecSections(ctx, r, attSpecID); err != nil || len(secs) != 1 || secs[0].Key != "overview" {
			t.Errorf("ListSpecSections = %+v err=%v", secs, err)
		}
		if rels, err := store.ListEntityRelationships(ctx, r, eStudentID); err != nil || len(rels) != 1 || !rels[0].Outgoing || rels[0].OtherName != "Course" {
			t.Errorf("ListEntityRelationships (student) = %+v err=%v", rels, err)
		}
		if rels, err := store.ListEntityRelationships(ctx, r, eCourseID); err != nil || len(rels) != 1 || rels[0].Outgoing {
			t.Errorf("ListEntityRelationships (course) = %+v err=%v", rels, err)
		}

		if types, err := store.ListSpecSectionTypes(ctx, r); err != nil || len(types) == 0 {
			t.Errorf("ListSpecSectionTypes = %d err=%v", len(types), err)
		}
		if types, err := store.ListEntitySectionTypes(ctx, r); err != nil || len(types) == 0 {
			t.Errorf("ListEntitySectionTypes = %d err=%v", len(types), err)
		}
		if _, ok, err := store.SpecSectionTypeByKey(ctx, r, "overview"); err != nil || !ok {
			t.Errorf("SpecSectionTypeByKey overview: ok=%v err=%v", ok, err)
		}
		if _, ok, _ := store.SpecSectionTypeByKey(ctx, r, "nope-type"); ok {
			t.Error("SpecSectionTypeByKey missing: want !ok")
		}
		if _, ok, err := store.EntitySectionTypeByKey(ctx, r, "purpose"); err != nil || !ok {
			t.Errorf("EntitySectionTypeByKey purpose: ok=%v err=%v", ok, err)
		}

		terms, err := store.ListGlossaryTerms(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		if len(terms) != 1 || terms[0].Slug != "attendance" || len(terms[0].Aliases) != 2 {
			t.Errorf("ListGlossaryTerms = %+v", terms)
		}
		if term, ok, err := store.GetGlossaryTerm(ctx, r, "attendance"); err != nil || !ok || len(term.Aliases) != 2 {
			t.Errorf("GetGlossaryTerm = %+v ok=%v", term, ok)
		}
		if _, ok, _ := store.GetGlossaryTerm(ctx, r, "nope"); ok {
			t.Error("GetGlossaryTerm missing: want !ok")
		}

		if edges, err := store.ListEdgesOfKind(ctx, r, "depends_on"); err != nil || len(edges) != 1 {
			t.Errorf("ListEdgesOfKind = %+v err=%v", edges, err)
		}
		if edges, err := store.ListAllEdges(ctx, r); err != nil || len(edges) != 1 {
			t.Errorf("ListAllEdges = %+v err=%v", edges, err)
		}
		if refs, err := store.ListEntityRefsByOwner(ctx, r, "spec", attSpecID); err != nil || len(refs) != 1 {
			t.Errorf("ListEntityRefsByOwner = %+v err=%v", refs, err)
		}
		if refs, err := store.ListEntityRefsFor(ctx, r, "entity", eStudentID); err != nil || len(refs) != 1 {
			t.Errorf("ListEntityRefsFor = %+v err=%v", refs, err)
		}
		if names, err := store.ListKeyEntities(ctx, r, attSpecID); err != nil || len(names) != 1 || names[0] != "Student" {
			t.Errorf("ListKeyEntities = %+v err=%v", names, err)
		}

		if targets, err := store.ListRefTargets(ctx, r); err != nil || len(targets) == 0 {
			t.Errorf("ListRefTargets empty err=%v", err)
		}
		if fields, err := store.ListProseFields(ctx, r); err != nil || len(fields) == 0 {
			t.Errorf("ListProseFields empty err=%v", err)
		}
		if hits, err := store.Search(ctx, r, "%Attendance%"); err != nil || len(hits) == 0 {
			t.Errorf("Search returned no hits err=%v", err)
		}

		if xs, err := store.ListExternalRefs(ctx, r, "", ""); err != nil || len(xs) < 2 {
			t.Errorf("ListExternalRefs all = %d err=%v", len(xs), err)
		}
		if xs, err := store.ListExternalRefs(ctx, r, "requirement", attReqID); err != nil || len(xs) != 1 {
			t.Errorf("ListExternalRefs filtered = %+v err=%v", xs, err)
		}
		if x, ok, err := store.GetExternalRef(ctx, r, beadRefID); err != nil || !ok || x.System != "beads" {
			t.Errorf("GetExternalRef = %+v ok=%v", x, ok)
		}
		if _, ok, _ := store.GetExternalRef(ctx, r, "nope"); ok {
			t.Error("GetExternalRef missing: want !ok")
		}
		if refs, err := store.ListExternalRefsForSubjects(ctx, r); err != nil || len(refs) != 1 || refs[0].System != "notion" {
			t.Errorf("ListExternalRefsForSubjects = %+v err=%v", refs, err)
		}

		if ms, err := store.ListMilestones(ctx, r); err != nil || len(ms) != 2 {
			t.Errorf("ListMilestones = %+v err=%v", ms, err)
		}
		if m, ok, err := store.GetMilestone(ctx, r, "m1"); err != nil || !ok || m.Name != "M1" {
			t.Errorf("GetMilestone m1 = %+v ok=%v", m, ok)
		}
		if _, ok, _ := store.GetMilestone(ctx, r, "nope"); ok {
			t.Error("GetMilestone missing: want !ok")
		}
		if id, ok, err := store.MilestoneIDBySlug(ctx, r, "m1"); err != nil || !ok || id != milestone1ID {
			t.Errorf("MilestoneIDBySlug = %q ok=%v", id, ok)
		}
		if _, ok, _ := store.MilestoneIDBySlug(ctx, r, "nope"); ok {
			t.Error("MilestoneIDBySlug missing: want !ok")
		}
		if caps, err := store.ListCapabilities(ctx, r); err != nil || len(caps) != 1 {
			t.Errorf("ListCapabilities = %+v err=%v", caps, err)
		}
		if c, ok, err := store.GetCapability(ctx, r, capID); err != nil || !ok || c.DomainSlug != "scheduling" {
			t.Errorf("GetCapability = %+v ok=%v", c, ok)
		}
		if _, ok, _ := store.GetCapability(ctx, r, "nope"); ok {
			t.Error("GetCapability missing: want !ok")
		}
		if dls, err := store.ListDeliverables(ctx, r); err != nil || len(dls) != 2 {
			t.Errorf("ListDeliverables = %+v err=%v", dls, err)
		}
		// del-1's second upsert set status=built and cleared its milestone_id.
		if d, ok, err := store.GetDeliverable(ctx, r, del1ID); err != nil || !ok || d.Status != "built" || d.MilestoneSlug != "" {
			t.Errorf("GetDeliverable = %+v ok=%v", d, ok)
		}
		if _, ok, _ := store.GetDeliverable(ctx, r, "nope"); ok {
			t.Error("GetDeliverable missing: want !ok")
		}
		// view-1's second upsert set title=View v2 and cleared its backing spec/route.
		if vs, err := store.ListPlanViews(ctx, r); err != nil || len(vs) != 1 || vs[0].Title != "View v2" || vs[0].DomainSlug != "scheduling" {
			t.Errorf("ListPlanViews = %+v err=%v", vs, err)
		}
		if v, ok, err := store.GetPlanView(ctx, r, viewID); err != nil || !ok || v.Title != "View v2" {
			t.Errorf("GetPlanView = %+v ok=%v", v, ok)
		}
		if _, ok, _ := store.GetPlanView(ctx, r, "nope"); ok {
			t.Error("GetPlanView missing: want !ok")
		}
		if p, err := store.ListCapabilityMilestonePairs(ctx, r); err != nil || len(p) != 1 || p[0].B != "m1" {
			t.Errorf("ListCapabilityMilestonePairs = %+v err=%v", p, err)
		}
		if p, err := store.ListCapabilityDeliverablePairs(ctx, r); err != nil || len(p) != 1 {
			t.Errorf("ListCapabilityDeliverablePairs = %+v err=%v", p, err)
		}
		if p, err := store.ListDeliverableViewPairs(ctx, r); err != nil || len(p) != 1 {
			t.Errorf("ListDeliverableViewPairs = %+v err=%v", p, err)
		}
		if p, err := store.ListDeliverableDependencyPairs(ctx, r); err != nil || len(p) != 1 {
			t.Errorf("ListDeliverableDependencyPairs = %+v err=%v", p, err)
		}
		if caps, delivs, views, err := store.PlanningCounts(ctx, r); err != nil || caps != 1 || delivs != 2 || views != 1 {
			t.Errorf("PlanningCounts = %d/%d/%d err=%v", caps, delivs, views, err)
		}

		if suites, err := store.ListTestSuites(ctx, r); err != nil || len(suites) < 2 {
			t.Errorf("ListTestSuites = %d err=%v", len(suites), err)
		}
		if s, ok, err := store.GetTestSuite(ctx, r, suiteID); err != nil || !ok || s.Name != "Suite A" {
			t.Errorf("GetTestSuite = %+v ok=%v", s, ok)
		}
		if _, ok, _ := store.GetTestSuite(ctx, r, "nope"); ok {
			t.Error("GetTestSuite missing: want !ok")
		}
		if cs, err := store.ListTestCases(ctx, r, suiteID); err != nil || len(cs) < 1 {
			t.Errorf("ListTestCases(suite) = %d err=%v", len(cs), err)
		}
		if cs, err := store.ListTestCases(ctx, r, ""); err != nil || len(cs) < 1 {
			t.Errorf("ListTestCases(all) = %d err=%v", len(cs), err)
		}
		if c, ok, err := store.GetTestCase(ctx, r, caseID); err != nil || !ok || c.Priority == nil || *c.Priority != 2 {
			t.Errorf("GetTestCase = %+v ok=%v", c, ok)
		}
		if _, ok, _ := store.GetTestCase(ctx, r, "nope"); ok {
			t.Error("GetTestCase missing: want !ok")
		}
		if steps, err := store.ListTestSteps(ctx, r, caseID); err != nil || len(steps) != 2 {
			t.Errorf("ListTestSteps = %d err=%v", len(steps), err)
		}
		if cfgs, err := store.ListConfigurations(ctx, r); err != nil || len(cfgs) < 1 {
			t.Errorf("ListConfigurations = %d err=%v", len(cfgs), err)
		}
		if c, ok, err := store.GetConfiguration(ctx, r, cfgID); err != nil || !ok || c.Name != "chrome" {
			t.Errorf("GetConfiguration = %+v ok=%v", c, ok)
		}
		if _, ok, _ := store.GetConfiguration(ctx, r, "nope"); ok {
			t.Error("GetConfiguration missing: want !ok")
		}
		if runs, err := store.ListTestRuns(ctx, r); err != nil || len(runs) < 1 {
			t.Errorf("ListTestRuns = %d err=%v", len(runs), err)
		}
		if run, ok, err := store.GetTestRun(ctx, r, runID); err != nil || !ok || run.MilestoneSlug != "m1" {
			t.Errorf("GetTestRun = %+v ok=%v", run, ok)
		}
		if _, ok, _ := store.GetTestRun(ctx, r, "nope"); ok {
			t.Error("GetTestRun missing: want !ok")
		}
		// The second UpsertTestResult (same deterministic id) updated status to failed
		// and cleared duration_ms.
		if results, err := store.ListTestResults(ctx, r, runID); err != nil || len(results) != 1 || results[0].Status != "failed" || results[0].DurationMs != nil {
			t.Errorf("ListTestResults = %+v err=%v", results, err)
		}

		if c, ok, err := store.GetChangesetByBranch(ctx, r, "changeset/test"); err != nil || !ok || c.ID != csID {
			t.Errorf("GetChangesetByBranch = %+v ok=%v", c, ok)
		}
		if _, ok, _ := store.GetChangesetByBranch(ctx, r, "nope"); ok {
			t.Error("GetChangesetByBranch missing: want !ok")
		}
		if cmts, err := store.ListComments(ctx, r, csID, store.CommentFilter{}); err != nil || len(cmts) != 2 {
			t.Errorf("ListComments = %d err=%v", len(cmts), err)
		}
		if cmts, err := store.ListComments(ctx, r, csID, store.CommentFilter{SubjectType: "requirement", SubjectID: attReqID}); err != nil || len(cmts) != 1 {
			t.Errorf("ListComments filtered = %d err=%v", len(cmts), err)
		}
		if cmts, err := store.ListComments(ctx, r, csID, store.CommentFilter{UnresolvedOnly: true}); err != nil || len(cmts) != 2 {
			t.Errorf("ListComments unresolved = %d err=%v", len(cmts), err)
		}
		if c, ok, err := store.GetComment(ctx, r, topCommentID); err != nil || !ok || c.Body != "top comment" {
			t.Errorf("GetComment = %+v ok=%v", c, ok)
		}
		if _, ok, _ := store.GetComment(ctx, r, "nope"); ok {
			t.Error("GetComment missing: want !ok")
		}
		if rvs, err := store.ListReviews(ctx, r, csID); err != nil || len(rvs) != 1 || rvs[0].Verdict != "approve" {
			t.Errorf("ListReviews = %+v err=%v", rvs, err)
		}
	})

	// ---------------------------------------------------------------- updates
	write(t, ws, func(ctx context.Context, tx store.Execer) error {
		if err := store.UpdateDomain(ctx, tx, schedID, "Scheduling v2", "desc2", "active"); err != nil {
			return err
		}
		if err := store.UpdateSpec(ctx, tx, attSpecID, "ATT Title v2", "active"); err != nil {
			return err
		}
		if err := store.UpdateRequirement(ctx, tx, store.Requirement{ID: attReqID, FRKey: "ATT-FR-001", Statement: "stmt v2", ContentStatus: "active", Notes: "note", Priority: iptr(1)}); err != nil {
			return err
		}
		if err := store.UpdateEntity(ctx, tx, eStudentID, "student v2", "active"); err != nil {
			return err
		}
		if err := store.UpdateGlossaryTerm(ctx, tx, glossID, "Attendance!", "def v2", "active"); err != nil {
			return err
		}
		if err := store.UpdateMilestone(ctx, tx, store.MilestoneRow{Slug: "m1", Name: "M1 v2", Sequence: iptr(1), Status: "active"}); err != nil {
			return err
		}
		if err := store.UpdateTestSuite(ctx, tx, store.TestSuiteRow{ID: suiteID, Name: "Suite A v2", Position: iptr(2)}); err != nil {
			return err
		}
		if err := store.UpdateTestCase(ctx, tx, store.TestCaseRow{ID: caseID, SuiteID: suiteID, Title: "Case v2", Status: "active"}); err != nil {
			return err
		}
		if err := store.UpdateConfiguration(ctx, tx, store.ConfigurationRow{ID: cfgID, Group: "os", Name: "linux"}); err != nil {
			return err
		}
		if err := store.UpdateTestRun(ctx, tx, store.TestRunRow{ID: runID, Title: "Run1 v2", Status: "complete"}, milestone1ID); err != nil {
			return err
		}

		// review-layer edits
		if err := store.SetChangesetStatus(ctx, tx, csID, enums.ChangesetOpen); err != nil {
			return err
		}
		if ok, err := store.SetCommentResolved(ctx, tx, topCommentID, true); err != nil || !ok {
			t.Errorf("SetCommentResolved: ok=%v err=%v", ok, err)
		}
		if ok, _ := store.SetCommentResolved(ctx, tx, "nope", true); ok {
			t.Error("SetCommentResolved missing: want !ok")
		}
		if ok, err := store.UpdateCommentBody(ctx, tx, topCommentID, "edited body"); err != nil || !ok {
			t.Errorf("UpdateCommentBody: ok=%v err=%v", ok, err)
		}
		if ok, _ := store.UpdateCommentBody(ctx, tx, "nope", "x"); ok {
			t.Error("UpdateCommentBody missing: want !ok")
		}
		if rv, err := store.UpsertReview(ctx, tx, csID, authorID, "request_changes", "please fix"); err != nil || rv.Verdict != "request_changes" {
			t.Errorf("UpsertReview update: %+v err=%v", rv, err)
		}
		if c, ok, err := store.DeleteComment(ctx, tx, replyCommentID); err != nil || !ok || c.ID != replyCommentID {
			t.Errorf("DeleteComment reply: %+v ok=%v err=%v", c, ok, err)
		}
		if _, ok, _ := store.DeleteComment(ctx, tx, "nope"); ok {
			t.Error("DeleteComment missing: want !ok")
		}

		// clear/unlink every junction
		for _, un := range []func() error{
			func() error { return store.UnlinkCapabilityMilestone(ctx, tx, capID, milestone1ID) },
			func() error { return store.ClearCapabilityMilestones(ctx, tx, capID) },
			func() error { return store.UnlinkCapabilityDeliverable(ctx, tx, capID, del1ID) },
			func() error { return store.ClearCapabilityDeliverables(ctx, tx, capID) },
			func() error { return store.UnlinkDeliverableView(ctx, tx, del1ID, viewID) },
			func() error { return store.ClearDeliverableViews(ctx, tx, del1ID) },
			func() error { return store.UnlinkDeliverableDependency(ctx, tx, del1ID, del2ID) },
			func() error { return store.ClearDeliverableDependencies(ctx, tx, del1ID) },
			func() error { return store.UnlinkRequirementTestCase(ctx, tx, attReqID, caseID) },
			func() error { return store.UnlinkRunConfiguration(ctx, tx, runID, cfgID) },
		} {
			if err := un(); err != nil {
				return err
			}
		}
		return nil
	})

	// ---------------------------------------------------------- read (update)
	read(t, ws, func(ctx context.Context, r store.Execer) {
		if d, _, _ := store.GetDomain(ctx, r, "scheduling"); d.Name != "Scheduling v2" {
			t.Errorf("UpdateDomain: name = %q", d.Name)
		}
		if s, _, _ := store.GetSpec(ctx, r, "ATT"); s.Title != "ATT Title v2" {
			t.Errorf("UpdateSpec: title = %q", s.Title)
		}
		if rq, _, _ := store.GetRequirement(ctx, r, "ATT-FR-001"); rq.Statement != "stmt v2" || rq.Notes != "note" || rq.Priority == nil || *rq.Priority != 1 {
			t.Errorf("UpdateRequirement: %+v", rq)
		}
		if e, _, _ := store.GetEntity(ctx, r, "Student"); e.Description != "student v2" {
			t.Errorf("UpdateEntity: desc = %q", e.Description)
		}
		if term, _, _ := store.GetGlossaryTerm(ctx, r, "attendance"); term.Term != "Attendance!" {
			t.Errorf("UpdateGlossaryTerm: term = %q", term.Term)
		}
		if m, _, _ := store.GetMilestone(ctx, r, "m1"); m.Name != "M1 v2" || m.Sequence == nil || *m.Sequence != 1 || m.Status != "active" {
			t.Errorf("UpdateMilestone: %+v", m)
		}
		if s, _, _ := store.GetTestSuite(ctx, r, suiteID); s.Name != "Suite A v2" {
			t.Errorf("UpdateTestSuite: name = %q", s.Name)
		}
		if c, _, _ := store.GetTestCase(ctx, r, caseID); c.Title != "Case v2" || c.Status != "active" {
			t.Errorf("UpdateTestCase: %+v", c)
		}
		if c, _, _ := store.GetConfiguration(ctx, r, cfgID); c.Group != "os" || c.Name != "linux" {
			t.Errorf("UpdateConfiguration: %+v", c)
		}
		if run, _, _ := store.GetTestRun(ctx, r, runID); run.Status != "complete" {
			t.Errorf("UpdateTestRun: status = %q", run.Status)
		}
		if c, _, _ := store.GetChangesetByBranch(ctx, r, "changeset/test"); c.Status != enums.ChangesetOpen {
			t.Errorf("SetChangesetStatus: status = %q", c.Status)
		}
		if c, _, _ := store.GetComment(ctx, r, topCommentID); c.Body != "edited body" || !c.Resolved {
			t.Errorf("comment after edits: %+v", c)
		}
		if cmts, _ := store.ListComments(ctx, r, csID, store.CommentFilter{}); len(cmts) != 1 {
			t.Errorf("ListComments after reply delete = %d, want 1", len(cmts))
		}
		if rvs, _ := store.ListReviews(ctx, r, csID); len(rvs) != 1 || rvs[0].Verdict != "request_changes" {
			t.Errorf("ListReviews after update = %+v", rvs)
		}
		for name, count := range map[string]int{
			"cap-mile":   mustLen(store.ListCapabilityMilestonePairs(ctx, r)),
			"cap-deliv":  mustLen(store.ListCapabilityDeliverablePairs(ctx, r)),
			"deliv-view": mustLen(store.ListDeliverableViewPairs(ctx, r)),
			"deliv-dep":  mustLen(store.ListDeliverableDependencyPairs(ctx, r)),
		} {
			if count != 0 {
				t.Errorf("junction %s not cleared: %d rows", name, count)
			}
		}
	})

	// ---------------------------------------------------------------- deletes
	write(t, ws, func(ctx context.Context, tx store.Execer) error {
		// throwaway domain/spec graph exercising the cascade cleaners.
		td, err := store.AddDomain(ctx, tx, store.Domain{Slug: "trash", Name: "Trash"})
		if err != nil {
			return err
		}
		tsp, err := store.AddSpec(ctx, tx, "trash", store.Spec{Prefix: "TRASH", Slug: "t", Title: "Trash Spec"})
		if err != nil {
			return err
		}
		if _, err := store.AddSpec(ctx, tx, "trash", store.Spec{Prefix: "TRASH2", Slug: "t2", Title: "Trash Spec 2"}); err != nil {
			return err
		}
		if _, err := store.AddRequirement(ctx, tx, "TRASH", store.Requirement{Statement: "to delete"}); err != nil {
			return err
		}
		if _, _, err := store.UpsertUserStory(ctx, tx, store.UserStory{SpecID: tsp.ID, Position: 1, Title: "s", Priority: 1}); err != nil {
			return err
		}
		if _, _, err := store.UpsertSpecSection(ctx, tx, tsp.ID, "notes", "x"); err != nil {
			return err
		}
		if _, err := store.UpsertEntityRef(ctx, tx, "spec", tsp.ID, "entity", eStudentID); err != nil {
			return err
		}
		if _, _, err := store.UpsertExternalRef(ctx, tx, "spec", tsp.ID, "sys", "x", ""); err != nil {
			return err
		}
		if _, err := store.AddEdgeByIDs(ctx, tx, "spec", tsp.ID, "relates", "domain", td.ID); err != nil {
			return err
		}

		if _, ok, err := store.DeleteRequirement(ctx, tx, "TRASH-FR-001"); err != nil || !ok {
			t.Errorf("DeleteRequirement: ok=%v err=%v", ok, err)
		}
		if _, ok, _ := store.DeleteRequirement(ctx, tx, "TRASH-FR-001"); ok {
			t.Error("DeleteRequirement again: want !ok")
		}
		if _, ok, err := store.DeleteSpec(ctx, tx, "TRASH"); err != nil || !ok {
			t.Errorf("DeleteSpec: ok=%v err=%v", ok, err)
		}
		if _, ok, _ := store.DeleteSpec(ctx, tx, "TRASH"); ok {
			t.Error("DeleteSpec again: want !ok")
		}
		if _, ok, err := store.DeleteDomain(ctx, tx, "trash"); err != nil || !ok {
			t.Errorf("DeleteDomain: ok=%v err=%v", ok, err)
		}
		if _, ok, _ := store.DeleteDomain(ctx, tx, "trash"); ok {
			t.Error("DeleteDomain again: want !ok")
		}

		// throwaway entity
		teID, _, err := store.UpsertEntity(ctx, tx, store.Entity{Name: "Trash", Description: "x"})
		if err != nil {
			return err
		}
		if _, _, err := store.UpsertEntitySection(ctx, tx, teID, "purpose", "x"); err != nil {
			return err
		}
		if _, err := store.UpsertEntityRelationship(ctx, tx, teID, eStudentID, "1:1", "j"); err != nil {
			return err
		}
		if _, err := store.UpsertEntityRef(ctx, tx, "entity", teID, "entity", eStudentID); err != nil {
			return err
		}
		if _, ok, err := store.DeleteEntity(ctx, tx, "Trash"); err != nil || !ok {
			t.Errorf("DeleteEntity: ok=%v err=%v", ok, err)
		}
		if _, ok, _ := store.DeleteEntity(ctx, tx, "Trash"); ok {
			t.Error("DeleteEntity again: want !ok")
		}

		// throwaway glossary term
		ttID, _, err := store.UpsertGlossaryTerm(ctx, tx, store.GlossaryTerm{Slug: "trash-term", Term: "T"})
		if err != nil {
			return err
		}
		if err := store.SetGlossaryAliases(ctx, tx, ttID, []string{"ta", "tb"}); err != nil {
			return err
		}
		if _, ok, err := store.DeleteGlossaryTerm(ctx, tx, "trash-term"); err != nil || !ok {
			t.Errorf("DeleteGlossaryTerm: ok=%v err=%v", ok, err)
		}
		if _, ok, _ := store.DeleteGlossaryTerm(ctx, tx, "trash-term"); ok {
			t.Error("DeleteGlossaryTerm again: want !ok")
		}

		// main edge + ref + section deletes
		if ok, err := store.DeleteEdgeByEndpoints(ctx, tx, "requirement", attReqID, "depends_on", "spec", attSpecID); err != nil || !ok {
			t.Errorf("DeleteEdgeByEndpoints: ok=%v err=%v", ok, err)
		}
		if ok, _ := store.DeleteEdgeByEndpoints(ctx, tx, "requirement", attReqID, "depends_on", "spec", attSpecID); ok {
			t.Error("DeleteEdgeByEndpoints again: want !ok")
		}
		if err := store.DeleteEntityRefsByOwner(ctx, tx, "spec", attSpecID); err != nil {
			return err
		}
		if ok, err := store.DeleteSpecSection(ctx, tx, attSpecID, "overview"); err != nil || !ok {
			t.Errorf("DeleteSpecSection: ok=%v err=%v", ok, err)
		}
		if ok, _ := store.DeleteSpecSection(ctx, tx, attSpecID, "overview"); ok {
			t.Error("DeleteSpecSection again: want !ok")
		}
		if ok, err := store.DeleteEntitySection(ctx, tx, eStudentID, "purpose"); err != nil || !ok {
			t.Errorf("DeleteEntitySection: ok=%v err=%v", ok, err)
		}
		if ok, _ := store.DeleteEntitySection(ctx, tx, eStudentID, "purpose"); ok {
			t.Error("DeleteEntitySection again: want !ok")
		}
		if err := store.DeleteSpecSectionsBySpec(ctx, tx, attSpecID); err != nil {
			return err
		}
		if err := store.DeleteEntitySectionsByEntity(ctx, tx, eStudentID); err != nil {
			return err
		}

		// external ref
		if ok, err := store.DeleteExternalRef(ctx, tx, beadRefID); err != nil || !ok {
			t.Errorf("DeleteExternalRef: ok=%v err=%v", ok, err)
		}
		if ok, _ := store.DeleteExternalRef(ctx, tx, "nope"); ok {
			t.Error("DeleteExternalRef missing: want !ok")
		}

		// testing rows
		if ok, err := store.DeleteTestStep(ctx, tx, step1ID); err != nil || !ok {
			t.Errorf("DeleteTestStep: ok=%v err=%v", ok, err)
		}
		if ok, _ := store.DeleteTestStep(ctx, tx, "nope"); ok {
			t.Error("DeleteTestStep missing: want !ok")
		}
		if ok, err := store.DeleteTestCase(ctx, tx, caseID); err != nil || !ok {
			t.Errorf("DeleteTestCase: ok=%v err=%v", ok, err)
		}
		if ok, err := store.DeleteTestRun(ctx, tx, runID); err != nil || !ok {
			t.Errorf("DeleteTestRun: ok=%v err=%v", ok, err)
		}
		if ok, err := store.DeleteConfiguration(ctx, tx, cfgID); err != nil || !ok {
			t.Errorf("DeleteConfiguration: ok=%v err=%v", ok, err)
		}
		if ok, err := store.DeleteTestSuite(ctx, tx, suiteID); err != nil || !ok {
			t.Errorf("DeleteTestSuite: ok=%v err=%v", ok, err)
		}

		// planning rows
		if ok, err := store.DeleteDeliverable(ctx, tx, del1ID); err != nil || !ok {
			t.Errorf("DeleteDeliverable: ok=%v err=%v", ok, err)
		}
		if _, err := store.DeleteDeliverable(ctx, tx, del2ID); err != nil {
			return err
		}
		if ok, _ := store.DeleteDeliverable(ctx, tx, "nope"); ok {
			t.Error("DeleteDeliverable missing: want !ok")
		}
		if ok, err := store.DeleteCapability(ctx, tx, capID); err != nil || !ok {
			t.Errorf("DeleteCapability: ok=%v err=%v", ok, err)
		}
		if ok, err := store.DeletePlanView(ctx, tx, viewID); err != nil || !ok {
			t.Errorf("DeletePlanView: ok=%v err=%v", ok, err)
		}
		if _, ok, err := store.DeleteMilestone(ctx, tx, "m1"); err != nil || !ok {
			t.Errorf("DeleteMilestone m1: ok=%v err=%v", ok, err)
		}
		if _, ok, err := store.DeleteMilestone(ctx, tx, "m2"); err != nil || !ok {
			t.Errorf("DeleteMilestone m2: ok=%v err=%v", ok, err)
		}
		if _, ok, _ := store.DeleteMilestone(ctx, tx, "nope"); ok {
			t.Error("DeleteMilestone missing: want !ok")
		}
		return nil
	})

	// ---------------------------------------------------------- read (delete)
	read(t, ws, func(ctx context.Context, r store.Execer) {
		if _, ok, _ := store.GetSpec(ctx, r, "TRASH"); ok {
			t.Error("GetSpec TRASH after delete: want !ok")
		}
		if _, ok, _ := store.GetDomain(ctx, r, "trash"); ok {
			t.Error("GetDomain trash after delete: want !ok")
		}
		if _, ok, _ := store.GetEntity(ctx, r, "Trash"); ok {
			t.Error("GetEntity Trash after delete: want !ok")
		}
		if _, ok, _ := store.GetGlossaryTerm(ctx, r, "trash-term"); ok {
			t.Error("GetGlossaryTerm trash-term after delete: want !ok")
		}
		if edges, _ := store.ListAllEdges(ctx, r); len(edges) != 0 {
			t.Errorf("ListAllEdges after delete = %d, want 0", len(edges))
		}
		if caps, delivs, views, _ := store.PlanningCounts(ctx, r); caps != 0 || delivs != 0 || views != 0 {
			t.Errorf("PlanningCounts after delete = %d/%d/%d, want 0/0/0", caps, delivs, views)
		}
		if ms, _ := store.ListMilestones(ctx, r); len(ms) != 0 {
			t.Errorf("ListMilestones after delete = %d, want 0", len(ms))
		}
		if _, ok, _ := store.GetTestSuite(ctx, r, suiteID); ok {
			t.Error("GetTestSuite after delete: want !ok")
		}
	})
}

func containsFunc[T any](s []T, pred func(T) bool) bool {
	for _, v := range s {
		if pred(v) {
			return true
		}
	}
	return false
}

func mustLen[T any](s []T, err error) int {
	if err != nil {
		return -1
	}
	return len(s)
}
