package generate_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/generate"
	"github.com/endermalkoc/cusp/internal/store"
	"github.com/endermalkoc/cusp/internal/testutil"
	"github.com/endermalkoc/cusp/internal/workspace"
)

// seeded carries the ids the generate tests need to reach back into specific documents.
type seeded struct {
	specID      string
	specDocPath string
	entityID    string
	entityDoc   string
}

// seedGraph writes a full cross-layer graph (domains, a nested-path spec with sections/
// stories/scenarios/groups/requirements, two entities, a glossary term, and the planning
// layer with junctions + external refs) in one commit, so Load has every branch to walk.
func seedGraph(t *testing.T, ws *workspace.Workspace) seeded {
	t.Helper()
	var s seeded
	dirty := []string{
		"req_domain", "req_spec", "req_spec_section", "req_user_story", "req_acceptance_scenario",
		"req_requirement_group", "req_requirement", "ent_entity", "ent_entity_section",
		"req_glossary_term", "req_glossary_alias", "plan_milestone", "plan_capability",
		"plan_capability_milestone", "plan_capability_deliverable", "plan_deliverable",
		"plan_deliverable_view", "plan_deliverable_dependency", "plan_view", "pub_external_ref",
	}
	err := app.Mutate(context.Background(), ws, app.MutateOpts{Summary: "seed", Changeset: "main", Actor: "seed-bot"},
		func(ctx context.Context, w *app.Write) error {
			for _, tbl := range dirty {
				w.MarkDirty(tbl)
			}
			tx := w.Tx

			enr, err := store.AddDomain(ctx, tx, store.Domain{Slug: "enrollment", Name: "Enrollment", Description: "Signing students up"})
			if err != nil {
				return err
			}
			if _, err := store.AddDomain(ctx, tx, store.Domain{Slug: "billing", Name: "Billing"}); err != nil {
				return err
			}

			// A spec nested under a sub-directory (student-detail/) exercises the nav tree's
			// directory grouping (insertDoc + navDir in the HTML sidebar).
			sp, err := store.AddSpec(ctx, tx, "enrollment", store.Spec{
				Prefix: "ENR", Path: "student-detail", Slug: "overview", Title: "Student Detail", Status: "active",
			})
			if err != nil {
				return err
			}
			s.specID = sp.ID
			s.specDocPath = store.SpecDocPath("enrollment", "student-detail", "overview")

			for _, sec := range []struct{ key, body string }{
				{"overview", "The student detail view. See [[TERM:student]]."},
				{"edge_cases", "What if the student was deleted?"},
				{"more_info", "Supplementary prose."},
			} {
				if _, _, err := store.UpsertSpecSection(ctx, tx, sp.ID, sec.key, sec.body); err != nil {
					return err
				}
			}

			storyID, _, err := store.UpsertUserStory(ctx, tx, store.UserStory{
				SpecID: sp.ID, Position: 1, Title: "View a student", Priority: 1,
				AsA: "tutor", IWant: "to see student detail", SoThat: "I can help",
				WhyPriority: "core", IndependentTest: "open a student page",
			})
			if err != nil {
				return err
			}
			if _, _, err := store.UpsertScenario(ctx, tx, store.Scenario{
				UserStoryID: storyID, Position: 1, Given: "a student exists", When: "I open it", Then: "I see details",
			}); err != nil {
				return err
			}

			if _, _, err := store.UpsertRequirementGroup(ctx, tx, sp.ID, 1, "Display", "Display rules."); err != nil {
				return err
			}
			for _, stmt := range []string{"The system MUST show the name.", "The system MUST show the email."} {
				if _, err := store.AddRequirement(ctx, tx, "ENR", store.Requirement{Statement: stmt}); err != nil {
					return err
				}
			}

			// Two entities: one top-level, one nested (entities/core/) for the entity nav tree.
			ent, _, err := store.UpsertEntity(ctx, tx, store.Entity{Name: "Student", Description: "A learner.", Status: "active"})
			if err != nil {
				return err
			}
			s.entityID = ent
			s.entityDoc = store.EntityDocPath("", "Student")
			if _, _, err := store.UpsertEntitySection(ctx, tx, ent, "purpose", "Represents a learner."); err != nil {
				return err
			}
			if _, _, err := store.UpsertEntity(ctx, tx, store.Entity{Name: "User Profile", Path: "core", Description: "Account data.", Status: "draft"}); err != nil {
				return err
			}

			termID, _, err := store.UpsertGlossaryTerm(ctx, tx, store.GlossaryTerm{
				Slug: "student", Term: "Student", Definition: "A person enrolled in a course.", DomainID: enr.ID,
			})
			if err != nil {
				return err
			}
			if err := store.SetGlossaryAliases(ctx, tx, termID, []string{"learner"}); err != nil {
				return err
			}

			// Planning layer.
			m0, err := store.AddMilestone(ctx, tx, store.MilestoneRow{Slug: "M0", Name: "M0"})
			if err != nil {
				return err
			}
			if _, err := store.UpsertCapability(ctx, tx, store.Capability{ID: "cap-root", Title: "Enroll Students", Level: "epic", DomainID: enr.ID}); err != nil {
				return err
			}
			if _, err := store.UpsertCapability(ctx, tx, store.Capability{ID: "cap-child", Title: "Bulk Import", Level: "capability", DomainID: enr.ID}); err != nil {
				return err
			}
			if err := store.SetCapabilityParent(ctx, tx, "cap-child", "cap-root"); err != nil {
				return err
			}
			if _, err := store.UpsertDeliverable(ctx, tx, store.Deliverable{ID: "d1", Title: "Signup form", Size: "M", Status: "built", AIReady: "yes", MilestoneID: m0.ID}); err != nil {
				return err
			}
			if _, err := store.UpsertDeliverable(ctx, tx, store.Deliverable{ID: "d2", Title: "Importer", Size: "L", Status: "proposed", AIReady: "no"}); err != nil {
				return err
			}
			if _, err := store.UpsertView(ctx, tx, store.View{ID: "v1", Title: "Signup Page", Route: "/signup", SpecID: sp.ID, DomainID: enr.ID}); err != nil {
				return err
			}
			if err := store.LinkCapabilityMilestone(ctx, tx, "cap-root", m0.ID); err != nil {
				return err
			}
			if err := store.LinkCapabilityDeliverable(ctx, tx, "cap-root", "d1"); err != nil {
				return err
			}
			if err := store.LinkDeliverableView(ctx, tx, "d1", "v1"); err != nil {
				return err
			}
			if err := store.LinkDeliverableDependency(ctx, tx, "d2", "d1"); err != nil {
				return err
			}
			if _, _, err := store.UpsertExternalRef(ctx, tx, "capability", "cap-root", "notion", "", "https://notion.example/cap-root"); err != nil {
				return err
			}
			if _, _, err := store.UpsertExternalRef(ctx, tx, "deliverable", "d1", "beads", "bd-1", ""); err != nil {
				return err
			}
			return nil
		})
	if err != nil {
		t.Fatalf("seedGraph: %v", err)
	}
	return s
}

func reader(t *testing.T, ws *workspace.Workspace) (store.Execer, func() error) {
	t.Helper()
	r, release, err := app.Reader(context.Background(), ws, "main")
	if err != nil {
		t.Fatalf("reader: %v", err)
	}
	return r, release
}

func TestGenerate_Markdown(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	s := seedGraph(t, ws)
	r, release := reader(t, ws)
	defer release()

	out := t.TempDir()
	st, err := generate.Generate(context.Background(), r, out, "md")
	if err != nil {
		t.Fatalf("Generate md: %v", err)
	}
	if st.Format != "markdown" {
		t.Errorf("format = %q, want markdown", st.Format)
	}
	if st.Specs != 1 || st.Entities != 2 || st.Glossary != 1 {
		t.Errorf("stats = %+v (want Specs=1 Entities=2 Glossary=1)", st)
	}
	if st.Planning != 4 {
		t.Errorf("planning pages = %d, want 4 (index+capabilities+deliverables+views)", st.Planning)
	}
	if st.Total() < 8 {
		t.Errorf("total = %d, unexpectedly small", st.Total())
	}

	// The nested-path spec landed at its reconstructed doc path, with real content.
	specPath := filepath.Join(out, filepath.FromSlash(s.specDocPath))
	b, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read spec md: %v", err)
	}
	specMD := string(b)
	for _, sub := range []string{"# Student Detail", "## Requirements", "ENR-FR-001", "View a student"} {
		if !strings.Contains(specMD, sub) {
			t.Errorf("generated spec md missing %q", sub)
		}
	}
	for _, rel := range []string{"index.md", "enrollment/index.md", "billing/index.md", "entities/index.md", "glossary.md", "planning/capabilities.md"} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected generated file %q: %v", rel, err)
		}
	}
}

func TestGenerate_JSONAndHTML(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	s := seedGraph(t, ws)
	r, release := reader(t, ws)
	defer release()

	// JSON.
	outJSON := t.TempDir()
	stJSON, err := generate.Generate(context.Background(), r, outJSON, "json")
	if err != nil {
		t.Fatalf("Generate json: %v", err)
	}
	if stJSON.Format != "json" || stJSON.Specs != 1 {
		t.Errorf("json stats = %+v", stJSON)
	}
	if _, err := os.Stat(filepath.Join(outJSON, "index.json")); err != nil {
		t.Errorf("index.json missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outJSON, "planning.json")); err != nil {
		t.Errorf("planning.json missing: %v", err)
	}

	// HTML — the nested spec's sidebar carries the directory grouping (navDir).
	outHTML := t.TempDir()
	stHTML, err := generate.Generate(context.Background(), r, outHTML, "html")
	if err != nil {
		t.Fatalf("Generate html: %v", err)
	}
	if stHTML.Format != "html" {
		t.Errorf("html format = %q", stHTML.Format)
	}
	if _, err := os.Stat(filepath.Join(outHTML, "assets", "style.css")); err != nil {
		t.Errorf("stylesheet missing: %v", err)
	}
	htmlPath := filepath.Join(outHTML, filepath.FromSlash(strings.TrimSuffix(s.specDocPath, ".md")+".html"))
	hb, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("read spec html: %v", err)
	}
	page := string(hb)
	for _, sub := range []string{"<!DOCTYPE html>", `class="nav-tree"`, `class="nav-dir"`, "Student Detail"} {
		if !strings.Contains(page, sub) {
			t.Errorf("spec html missing %q", sub)
		}
	}

	if _, err := generate.Generate(context.Background(), r, t.TempDir(), "bogus"); err == nil {
		t.Error("expected error for unknown format")
	}
}

func TestSync_FullThenIncremental(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	s := seedGraph(t, ws)
	r, release := reader(t, ws)
	defer release()

	ctx := context.Background()
	outMD := t.TempDir()
	outJSON := t.TempDir()
	targets := []generate.Target{{Format: "md", OutDir: outMD}, {Format: "json", OutDir: outJSON}}

	// Full sync writes everything for both formats and records manifests.
	full, err := generate.Sync(ctx, r, targets, generate.DirtySet{Full: true})
	if err != nil {
		t.Fatalf("Sync full: %v", err)
	}
	if !full.Full || full.Written == 0 {
		t.Fatalf("full sync wrote nothing: %+v", full)
	}
	if len(full.Formats) != 2 {
		t.Errorf("full sync formats = %v", full.Formats)
	}

	// Re-running the full sync with unchanged data is a no-op (content hashes match).
	again, err := generate.Sync(ctx, r, targets, generate.DirtySet{Full: true})
	if err != nil {
		t.Fatalf("Sync full again: %v", err)
	}
	if again.Written != 0 || again.Removed != 0 {
		t.Errorf("idempotent full sync should write/remove nothing: %+v", again)
	}

	// Incremental sync of just the one spec re-renders only its per-document files (kept via
	// keepDocs) and never touches index/glossary rollups.
	inc, err := generate.Sync(ctx, r, targets, generate.DirtySet{SpecIDs: []string{s.specID}})
	if err != nil {
		t.Fatalf("Sync incremental: %v", err)
	}
	if inc.Full {
		t.Error("incremental sync should not be Full")
	}
	if inc.Written != 0 {
		t.Errorf("unchanged incremental sync should write nothing, wrote %d", inc.Written)
	}
	// The rollup index page still exists on disk from the full sync (incremental never removed it).
	if _, err := os.Stat(filepath.Join(outMD, "index.md")); err != nil {
		t.Errorf("index.md should survive incremental sync: %v", err)
	}
}

func TestRenderSpecDoc(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	s := seedGraph(t, ws)
	r, release := reader(t, ws)
	defer release()
	ctx := context.Background()

	md, err := generate.RenderSpecDoc(ctx, r, s.specID, s.specDocPath, "md")
	if err != nil {
		t.Fatalf("RenderSpecDoc md: %v", err)
	}
	if !strings.Contains(md, "# Student Detail") || !strings.Contains(md, "ENR-FR-001") {
		t.Errorf("rendered spec md wrong:\n%s", md)
	}

	page, err := generate.RenderSpecDoc(ctx, r, s.specID, s.specDocPath, "html")
	if err != nil {
		t.Fatalf("RenderSpecDoc html: %v", err)
	}
	if !strings.Contains(page, "<!DOCTYPE html>") || !strings.Contains(page, `class="embedded"`) {
		t.Errorf("rendered spec html wrong")
	}

	if _, err := generate.RenderSpecDoc(ctx, r, s.specID, "wrong/path.md", "md"); err == nil {
		t.Error("expected error for wrong doc path")
	}
}

func TestRenderEntityDoc(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	s := seedGraph(t, ws)
	r, release := reader(t, ws)
	defer release()
	ctx := context.Background()

	md, err := generate.RenderEntityDoc(ctx, r, s.entityID, s.entityDoc, "md")
	if err != nil {
		t.Fatalf("RenderEntityDoc md: %v", err)
	}
	if !strings.Contains(md, "# Student") || !strings.Contains(md, "Represents a learner.") {
		t.Errorf("rendered entity md wrong:\n%s", md)
	}

	page, err := generate.RenderEntityDoc(ctx, r, s.entityID, s.entityDoc, "html")
	if err != nil {
		t.Fatalf("RenderEntityDoc html: %v", err)
	}
	if !strings.Contains(page, "Student · Cusp") {
		t.Errorf("rendered entity html wrong")
	}
}
