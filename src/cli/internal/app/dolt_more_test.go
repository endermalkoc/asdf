package app_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/store"
	"github.com/endermalkoc/cusp/internal/testutil"
	"github.com/endermalkoc/cusp/internal/workspace"
)

// --- CheckEdgeAcyclic -----------------------------------------------------

func TestCheckEdgeAcyclic(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	req1ID, req2ID := seedGraph(t, ws)

	// Existing edge: req1 --depends_on--> req2.
	mutate(t, ws, "main", func(ctx context.Context, w *app.Write) error {
		_, e := store.AddEdgeByIDs(ctx, w.Tx, "requirement", req1ID, "depends_on", "requirement", req2ID)
		w.MarkDirty("req_edge")
		return e
	})

	ctx := context.Background()
	r, release, err := app.Reader(ctx, ws, "main")
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// A self-loop is rejected outright.
	if err := app.CheckEdgeAcyclic(ctx, r, "requirement", req1ID, "depends_on", "requirement", req1ID, "R1", "R1"); err == nil {
		t.Error("expected a self-loop to be rejected")
	}

	// req2 --depends_on--> req1 would close the cycle (req1 already reaches req2).
	if err := app.CheckEdgeAcyclic(ctx, r, "requirement", req2ID, "depends_on", "requirement", req1ID, "R2", "R1"); err == nil {
		t.Error("expected the back-edge to be rejected as a cycle")
	}

	// The same direction as the existing edge does not create a cycle.
	if err := app.CheckEdgeAcyclic(ctx, r, "requirement", req1ID, "depends_on", "requirement", req2ID, "R1", "R2"); err != nil {
		t.Errorf("acyclic edge should be allowed: %v", err)
	}

	// A cycle-permitting kind is a no-op even when it would form a loop.
	if err := app.CheckEdgeAcyclic(ctx, r, "requirement", req2ID, "references", "requirement", req1ID, "R2", "R1"); err != nil {
		t.Errorf("references edges permit cycles: %v", err)
	}
}

// --- Remote configuration -------------------------------------------------

func TestRemoteConfig(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	ctx := context.Background()

	// A fresh workspace has no remotes.
	if list, err := app.RemoteList(ctx, ws); err != nil || len(list) != 0 {
		t.Fatalf("fresh workspace remotes: list=%+v err=%v", list, err)
	}

	remoteURL := "file://" + filepath.Join(t.TempDir(), "remote")
	if err := app.RemoteAdd(ctx, ws, "origin", remoteURL); err != nil {
		t.Fatalf("remote add: %v", err)
	}
	list, err := app.RemoteList(ctx, ws)
	if err != nil {
		t.Fatalf("remote list: %v", err)
	}
	found := false
	for _, rr := range list {
		if rr.Name == "origin" {
			found = true
		}
	}
	if !found {
		t.Fatalf("added remote not listed: %+v", list)
	}

	if err := app.RemoteRemove(ctx, ws, "origin"); err != nil {
		t.Fatalf("remote remove: %v", err)
	}
	if list, err := app.RemoteList(ctx, ws); err != nil || len(list) != 0 {
		t.Fatalf("after remove: list=%+v err=%v", list, err)
	}
}

// --- Flatten / GarbageCollect ---------------------------------------------

func TestFlattenAndGarbageCollect(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	addDomain(t, ws, "main", "att")
	addDomain(t, ws, "main", "sched")
	before := testutil.CommitCount(t, ws, "main")

	ctx := context.Background()
	if err := app.Flatten(ctx, ws); err != nil {
		t.Fatalf("flatten: %v", err)
	}
	// Flatten squashes history (it keeps the root and collapses the rest), so the commit
	// count strictly shrinks.
	after := testutil.CommitCount(t, ws, "main")
	if after >= before {
		t.Fatalf("flatten did not reduce history: %d -> %d", before, after)
	}
	// The flattened data is still present.
	if !domainSlugs(t, ws, "main")["att"] || !domainSlugs(t, ws, "main")["sched"] {
		t.Fatal("flatten lost data")
	}

	// A standalone GC just reclaims space; it must succeed and change nothing observable.
	if err := app.GarbageCollect(ctx, ws); err != nil {
		t.Fatalf("garbage collect: %v", err)
	}
	if got := testutil.CommitCount(t, ws, "main"); got != after {
		t.Fatalf("gc changed history: %d -> %d", after, got)
	}
}

// --- AutoGenerate dirty classification ------------------------------------

// TestAutoGenerate_ClassifiesDirtyTables enables auto-generation and drives one mutation per
// dirty-table class so AutoGenerate's classifier (classifyDirty → diffOwnerIDs / scenarioSpecIDs /
// requirementDirty / queryStrings) runs over real Dolt diffs.
func TestAutoGenerate_ClassifiesDirtyTables(t *testing.T) {
	ws := testutil.NewWorkspace(t)

	// Enable auto-gen with a markdown target rendered into a temp dir.
	cfg := &workspace.Config{}
	cfg.Generate.Enabled = true
	cfg.Generate.Formats = []workspace.FormatConfig{{Format: "markdown", Out: t.TempDir()}}
	if err := ws.SaveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	// (a) A domain add is an "other" table → a full rebuild.
	addDomain(t, ws, "main", "att")

	// (b) A spec add is also a full rebuild.
	mutate(t, ws, "main", func(ctx context.Context, w *app.Write) error {
		_, e := store.AddSpec(ctx, w.Tx, "att", store.Spec{Prefix: "ATT", Slug: "s", Title: "S"})
		w.MarkDirty("req_spec")
		return e
	})

	// (c) A requirement insert is a structural change → full rebuild (requirementDirty structural).
	var reqID string
	mutate(t, ws, "main", func(ctx context.Context, w *app.Write) error {
		r, e := store.AddRequirement(ctx, w.Tx, "ATT", store.Requirement{Statement: "Original statement."})
		reqID = r.ID
		w.MarkDirty("req_requirement")
		return e
	})

	// (d) A requirement statement edit (fr_key unchanged) is local → requirementDirty modified path.
	mutate(t, ws, "main", func(ctx context.Context, w *app.Write) error {
		r, ok, e := store.GetRequirement(ctx, w.Tx, "ATT-FR-001")
		if e != nil || !ok {
			t.Fatalf("get requirement: ok=%v err=%v", ok, e)
		}
		r.Statement = "Edited statement."
		if e := store.UpdateRequirement(ctx, w.Tx, r); e != nil {
			return e
		}
		w.MarkDirty("req_requirement")
		return nil
	})
	_ = reqID

	// (e) A user-story add touches only a local spec table → diffOwnerIDs / queryStrings.
	var specID, storyID string
	mutate(t, ws, "main", func(ctx context.Context, w *app.Write) error {
		var ok bool
		var e error
		specID, ok, e = store.SpecIDByPrefix(ctx, w.Tx, "ATT")
		if e != nil || !ok {
			t.Fatalf("spec id: ok=%v err=%v", ok, e)
		}
		storyID, _, e = store.UpsertUserStory(ctx, w.Tx, store.UserStory{
			SpecID: specID, Position: 1, Title: "Story", Priority: 2, AsA: "user", IWant: "x", SoThat: "y",
		})
		w.MarkDirty("req_user_story")
		return e
	})
	if storyID == "" {
		t.Fatal("user story id not captured")
	}

	// (f) An acceptance-scenario add rolls up through the story → scenarioSpecIDs / queryStrings.
	mutate(t, ws, "main", func(ctx context.Context, w *app.Write) error {
		_, _, e := store.UpsertScenario(ctx, w.Tx, store.Scenario{
			UserStoryID: storyID, Position: 1, Given: "a", When: "b", Then: "c",
		})
		w.MarkDirty("req_acceptance_scenario")
		return e
	})

	// All mutations committed cleanly on main.
	if dirty := testutil.DirtyTables(t, ws, "main"); len(dirty) != 0 {
		t.Fatalf("auto-gen mutations left a dirty working set: %v", dirty)
	}
}
