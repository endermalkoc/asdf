// Package integration exercises the command contract end to end against a real (owned) Dolt
// server via the testutil harness. These tests need the `dolt` binary on PATH; they skip under
// -short or when it is absent. They drive the same app.Mutate / store / versioncontrolops
// primitives the CLI commands use, asserting the guarantees only a real DB can prove: one commit
// per change with a clean working set, validation-before-write, branch-scoped reads, the changeset
// round-trip, and review rows persisting on main across a merge.
package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/enums"
	"github.com/endermalkoc/cusp/internal/ids"
	"github.com/endermalkoc/cusp/internal/storage/versioncontrolops"
	"github.com/endermalkoc/cusp/internal/store"
	"github.com/endermalkoc/cusp/internal/testutil"
	"github.com/endermalkoc/cusp/internal/workspace"
)

const botActor = "integration-bot"

// --- Scenario 1: init produces a clean, committed workspace -----------------

func TestInit_CleanAndCommitted(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	if got := testutil.CommitCount(t, ws, "main"); got < 1 {
		t.Fatalf("expected >= 1 commit after init, got %d", got)
	}
	if dirty := testutil.DirtyTables(t, ws, "main"); len(dirty) != 0 {
		t.Fatalf("expected clean working set after init, got dirty: %v", dirty)
	}
	// Schema is applied: a known table is queryable.
	if slugs := domainSlugs(t, ws, "main"); len(slugs) != 0 {
		t.Fatalf("expected 0 domains in a fresh workspace, got %v", slugs)
	}
}

// --- Scenario 2: each add is exactly one commit, clean, attributed ----------

func TestAdd_IsExactlyOneCommit(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	before := testutil.CommitCount(t, ws, "main")

	addDomain(t, ws, "main", "att")

	if after := testutil.CommitCount(t, ws, "main"); after != before+1 {
		t.Fatalf("expected exactly one new commit, got %d -> %d", before, after)
	}
	if dirty := testutil.DirtyTables(t, ws, "main"); len(dirty) != 0 {
		t.Fatalf("add left a dirty working set: %v", dirty)
	}
	if slugs := domainSlugs(t, ws, "main"); !slugs["att"] {
		t.Fatalf("added domain not present: %v", slugs)
	}
	if author := testutil.LatestCommitAuthor(t, ws, "main"); !strings.Contains(author, botActor) {
		t.Fatalf("commit not attributed to %q: %q", botActor, author)
	}
}

// --- Scenario 3: validation rejects before any write ------------------------

func TestValidation_RejectsBeforeWrite(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	before := testutil.CommitCount(t, ws, "main")

	wantErr := fmt.Errorf("nope")
	err := app.Mutate(context.Background(), ws, app.MutateOpts{
		Summary:   "should not commit",
		Changeset: "main",
		Actor:     botActor,
		Validate:  func(ctx context.Context, r store.Execer) error { return wantErr },
	}, func(ctx context.Context, w *app.Write) error {
		_, e := store.AddDomain(ctx, w.Tx, store.Domain{Slug: "ghost", Name: "Ghost"})
		w.MarkDirty("req_domain")
		return e
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if after := testutil.CommitCount(t, ws, "main"); after != before {
		t.Fatalf("failed validation still committed: %d -> %d", before, after)
	}
	if slugs := domainSlugs(t, ws, "main"); slugs["ghost"] {
		t.Fatal("failed validation still wrote a row")
	}
}

// --- Scenario 3b: dry-run rolls back ----------------------------------------

func TestDryRun_RollsBack(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	before := testutil.CommitCount(t, ws, "main")

	err := app.Mutate(context.Background(), ws, app.MutateOpts{
		Summary:   "dry run",
		Changeset: "main",
		Actor:     botActor,
		DryRun:    true,
	}, func(ctx context.Context, w *app.Write) error {
		_, e := store.AddDomain(ctx, w.Tx, store.Domain{Slug: "temp", Name: "Temp"})
		w.MarkDirty("req_domain")
		return e
	})
	if err != nil {
		t.Fatalf("dry run returned error: %v", err)
	}
	if after := testutil.CommitCount(t, ws, "main"); after != before {
		t.Fatalf("dry run committed: %d -> %d", before, after)
	}
	if slugs := domainSlugs(t, ws, "main"); slugs["temp"] {
		t.Fatal("dry run persisted a row")
	}
}

// --- Scenario 4: reads honor the active changeset (branch isolation) --------

func TestReads_HonorChangesetBranch(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	branch := startChangeset(t, ws, "isolate")

	addDomain(t, ws, branch, "onbranch")

	if slugs := domainSlugs(t, ws, "main"); slugs["onbranch"] {
		t.Fatal("changeset edit leaked onto main")
	}
	if slugs := domainSlugs(t, ws, branch); !slugs["onbranch"] {
		t.Fatal("changeset edit not visible on its own branch")
	}
}

// --- Scenario 5+6: round-trip; review rows on main survive the merge --------

func TestRoundTrip_ReviewRowsSurviveMerge(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	branch := startChangeset(t, ws, "revise")
	addDomain(t, ws, branch, "brdom")

	// A review comment is written to main (not the branch) — it must survive the merge.
	cs := comment(t, ws, branch, "looks good")

	// Merge the changeset into main (the same primitive `cusp changeset merge` uses).
	mergeChangeset(t, ws, branch)

	// main now carries the branch's data...
	if slugs := domainSlugs(t, ws, "main"); !slugs["brdom"] {
		t.Fatal("merge did not bring the branch's domain to main")
	}
	// ...and the review comment is still there.
	r, release, err := app.Reader(context.Background(), ws, "main")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	comments, err := store.ListComments(context.Background(), r, cs.ChangesetID, store.CommentFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 || comments[0].Body != "looks good" {
		t.Fatalf("review comment did not survive merge: %+v", comments)
	}
}

// --- Scenario 7: deterministic upsert is idempotent -------------------------

func TestUpsert_Idempotent(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	upsert := func() bool {
		var inserted bool
		err := app.Mutate(context.Background(), ws, app.MutateOpts{Summary: "upsert", Changeset: "main", Actor: botActor},
			func(ctx context.Context, w *app.Write) error {
				_, ins, e := store.UpsertDomain(ctx, w.Tx, store.Domain{Slug: "up", Name: "Up"})
				inserted = ins
				w.MarkDirty("req_domain")
				return e
			})
		if err != nil {
			t.Fatalf("upsert: %v", err)
		}
		return inserted
	}
	if !upsert() {
		t.Fatal("first upsert should insert")
	}
	if upsert() {
		t.Fatal("second upsert should update, not insert")
	}
	// Exactly one row for the slug.
	n := 0
	for s := range domainSlugs(t, ws, "main") {
		if s == "up" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("expected 1 'up' domain, got %d", n)
	}
}

// --- helpers ----------------------------------------------------------------

// addDomain writes a domain through the real mutation contract on the given branch ("" = active/main).
func addDomain(t *testing.T, ws *workspace.Workspace, changeset, slug string) {
	t.Helper()
	err := app.Mutate(context.Background(), ws, app.MutateOpts{
		Summary:   "add domain " + slug,
		Changeset: changeset,
		Actor:     botActor,
	}, func(ctx context.Context, w *app.Write) error {
		_, e := store.AddDomain(ctx, w.Tx, store.Domain{Slug: slug, Name: slug})
		if e != nil {
			return e
		}
		w.MarkDirty("req_domain")
		return nil
	})
	if err != nil {
		t.Fatalf("addDomain %q: %v", slug, err)
	}
}

// domainSlugs reads the domain slugs visible on `branch` (via the branch-pinned reader).
func domainSlugs(t *testing.T, ws *workspace.Workspace, branch string) map[string]bool {
	t.Helper()
	ctx := context.Background()
	r, release, err := app.Reader(ctx, ws, branch)
	if err != nil {
		t.Fatalf("reader %s: %v", branch, err)
	}
	defer release()
	ds, err := store.ListDomains(ctx, r)
	if err != nil {
		t.Fatalf("list domains: %v", err)
	}
	m := map[string]bool{}
	for _, d := range ds {
		m[d.Slug] = true
	}
	return m
}

// startChangeset creates a changeset branch and its rev_changeset row on main — mirroring what
// `cusp changeset start` does — and returns the branch name.
func startChangeset(t *testing.T, ws *workspace.Workspace, title string) string {
	t.Helper()
	ctx := context.Background()
	branch := "changeset/" + title
	conn, err := ws.Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := versioncontrolops.CheckoutBranch(ctx, conn, "main"); err != nil {
		t.Fatal(err)
	}
	entries, err := versioncontrolops.Log(ctx, conn, 1)
	if err != nil || len(entries) == 0 {
		t.Fatalf("head: %v", err)
	}
	base := entries[0].Hash
	if err := versioncontrolops.CreateBranch(ctx, conn, branch); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	authorID, err := store.SeedActor(ctx, conn, testutil.TestActor.Handle, testutil.TestActor.Name)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := conn.ExecContext(ctx,
		"INSERT INTO `rev_changeset` (id,title,description,author_id,status,branch,base_commit,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?)",
		ids.New(), title, "", authorID, enums.ChangesetDraft, branch, base, now, now); err != nil {
		t.Fatalf("insert rev_changeset: %v", err)
	}
	if err := versioncontrolops.StageAndCommit(ctx, conn, map[string]bool{"rev_changeset": true, "rev_actor": true},
		"open changeset "+branch, testutil.TestActor.CommitAuthorString()); err != nil {
		t.Fatalf("commit changeset: %v", err)
	}
	return branch
}

// comment writes a review comment on main against the changeset `branch` (as `cusp comment add`).
func comment(t *testing.T, ws *workspace.Workspace, branch, body string) store.CommentRow {
	t.Helper()
	var c store.CommentRow
	err := app.Mutate(context.Background(), ws, app.MutateOpts{Summary: "comment", Changeset: "main", Actor: botActor},
		func(ctx context.Context, w *app.Write) error {
			cs, ok, e := store.GetChangesetByBranch(ctx, w.Tx, branch)
			if e != nil {
				return e
			}
			if !ok {
				return fmt.Errorf("no changeset %s", branch)
			}
			authorID, e := store.SeedActor(ctx, w.Tx, w.Actor.Handle, w.Actor.Name)
			if e != nil {
				return e
			}
			c, e = store.AddComment(ctx, w.Tx, cs.ID, authorID, "", body, "", "", "")
			if e != nil {
				return e
			}
			w.MarkDirty("rev_comment")
			w.MarkDirty("rev_actor")
			return nil
		})
	if err != nil {
		t.Fatalf("comment: %v", err)
	}
	return c
}

// mergeChangeset merges `branch` into main via the same primitive the merge command uses.
func mergeChangeset(t *testing.T, ws *workspace.Workspace, branch string) {
	t.Helper()
	ctx := context.Background()
	conn, err := ws.Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := versioncontrolops.CheckoutBranch(ctx, conn, "main"); err != nil {
		t.Fatal(err)
	}
	conflicts, err := versioncontrolops.Merge(ctx, conn, branch, testutil.TestActor.CommitAuthorString())
	if err != nil {
		t.Fatalf("merge %s: %v", branch, err)
	}
	if len(conflicts) > 0 {
		t.Fatalf("unexpected merge conflicts: %+v", conflicts)
	}
}
