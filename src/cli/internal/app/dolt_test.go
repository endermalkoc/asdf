package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/refs"
	"github.com/endermalkoc/cusp/internal/storage/versioncontrolops"
	"github.com/endermalkoc/cusp/internal/store"
	"github.com/endermalkoc/cusp/internal/testutil"
	"github.com/endermalkoc/cusp/internal/workspace"
)

const testActor = "app-test-bot"

// mutate runs body through the real mutation contract on the given branch.
func mutate(t *testing.T, ws *workspace.Workspace, changeset string, body func(ctx context.Context, w *app.Write) error) {
	t.Helper()
	if err := app.Mutate(context.Background(), ws, app.MutateOpts{
		Summary:   "test mutation",
		Changeset: changeset,
		Actor:     testActor,
	}, body); err != nil {
		t.Fatalf("mutate: %v", err)
	}
}

// addDomain writes a domain on the given branch.
func addDomain(t *testing.T, ws *workspace.Workspace, changeset, slug string) {
	t.Helper()
	mutate(t, ws, changeset, func(ctx context.Context, w *app.Write) error {
		_, e := store.AddDomain(ctx, w.Tx, store.Domain{Slug: slug, Name: strings.ToUpper(slug)})
		w.MarkDirty("req_domain")
		return e
	})
}

// seedGraph builds domain "att", spec "ATT", and two requirements on main; it returns their ids.
func seedGraph(t *testing.T, ws *workspace.Workspace) (req1ID, req2ID string) {
	t.Helper()
	ctx := context.Background()
	mutate(t, ws, "main", func(ctx context.Context, w *app.Write) error {
		if _, e := store.AddDomain(ctx, w.Tx, store.Domain{Slug: "att", Name: "Attendance"}); e != nil {
			return e
		}
		w.MarkDirty("req_domain")
		if _, e := store.AddSpec(ctx, w.Tx, "att", store.Spec{
			Prefix: "ATT", Slug: "take-attendance", Path: "scheduling", Title: "Take Attendance",
		}); e != nil {
			return e
		}
		w.MarkDirty("req_spec")
		r1, e := store.AddRequirement(ctx, w.Tx, "ATT", store.Requirement{Statement: "The system records attendance."})
		if e != nil {
			return e
		}
		req1ID = r1.ID
		r2, e := store.AddRequirement(ctx, w.Tx, "ATT", store.Requirement{Statement: "The system reports attendance."})
		if e != nil {
			return e
		}
		req2ID = r2.ID
		w.MarkDirty("req_requirement")
		return nil
	})
	if req1ID == "" || req2ID == "" {
		t.Fatal("seedGraph: requirement ids not captured")
	}
	_ = ctx
	return req1ID, req2ID
}

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

// --- Mutate --------------------------------------------------------------

func TestMutate_SuccessOneCommitCleanAttributed(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	before := testutil.CommitCount(t, ws, "main")

	addDomain(t, ws, "main", "att")

	if after := testutil.CommitCount(t, ws, "main"); after != before+1 {
		t.Fatalf("expected exactly one new commit, got %d -> %d", before, after)
	}
	if dirty := testutil.DirtyTables(t, ws, "main"); len(dirty) != 0 {
		t.Fatalf("mutation left a dirty working set: %v", dirty)
	}
	if !domainSlugs(t, ws, "main")["att"] {
		t.Fatal("added domain not present")
	}
	if author := testutil.LatestCommitAuthor(t, ws, "main"); !strings.Contains(author, testActor) {
		t.Fatalf("commit not attributed to %q: %q", testActor, author)
	}
}

func TestMutate_ValidateRejectsBeforeWrite(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	before := testutil.CommitCount(t, ws, "main")

	err := app.Mutate(context.Background(), ws, app.MutateOpts{
		Summary:   "should not commit",
		Changeset: "main",
		Actor:     testActor,
		Validate:  func(ctx context.Context, r store.Execer) error { return app.ValidationFailed(context.DeadlineExceeded) },
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
	if domainSlugs(t, ws, "main")["ghost"] {
		t.Fatal("failed validation still wrote a row")
	}
}

func TestMutate_DryRunRollsBack(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	before := testutil.CommitCount(t, ws, "main")

	err := app.Mutate(context.Background(), ws, app.MutateOpts{
		Summary:   "dry run",
		Changeset: "main",
		Actor:     testActor,
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
	if domainSlugs(t, ws, "main")["temp"] {
		t.Fatal("dry run persisted a row")
	}
}

// --- ResolveBranch / Reader ----------------------------------------------

func TestResolveBranch(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	if got := app.ResolveBranch(ws, "explicit"); got != "explicit" {
		t.Errorf("explicit changeset should win: %q", got)
	}
	if got := app.ResolveBranch(ws, ""); got != "main" {
		t.Errorf("default with no active changeset should be main: %q", got)
	}
	if err := ws.SetActiveChangeset("changeset/work"); err != nil {
		t.Fatal(err)
	}
	if got := app.ResolveBranch(ws, ""); got != "changeset/work" {
		t.Errorf("active changeset should be honored: %q", got)
	}
	if got := app.ResolveBranch(ws, "override"); got != "override" {
		t.Errorf("explicit still overrides active: %q", got)
	}
	if err := ws.ClearActiveChangeset(); err != nil {
		t.Fatal(err)
	}
	if got := app.ResolveBranch(ws, ""); got != "main" {
		t.Errorf("cleared active should fall back to main: %q", got)
	}
}

func TestReader_BranchIsolation(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	if err := app.CreateBranch(context.Background(), ws, "changeset/iso"); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	addDomain(t, ws, "changeset/iso", "onbranch")

	if domainSlugs(t, ws, "main")["onbranch"] {
		t.Fatal("changeset edit leaked onto main")
	}
	if !domainSlugs(t, ws, "changeset/iso")["onbranch"] {
		t.Fatal("changeset edit not visible on its own branch")
	}
}

func TestReader_CommitHash(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	addDomain(t, ws, "main", "att")

	// Grab main's head commit hash.
	ctx := context.Background()
	conn, err := ws.Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := versioncontrolops.CheckoutBranch(ctx, conn, "main"); err != nil {
		t.Fatal(err)
	}
	entries, err := versioncontrolops.Log(ctx, conn, 1)
	if err != nil || len(entries) == 0 {
		t.Fatalf("log: %v", err)
	}
	hash := entries[0].Hash
	conn.Close()

	// Reading at the commit hash exercises the read-only revision-database path.
	r, release, err := app.Reader(ctx, ws, hash)
	if err != nil {
		t.Fatalf("reader at commit %s: %v", hash, err)
	}
	ds, err := store.ListDomains(ctx, r)
	if err != nil {
		t.Fatalf("list at commit: %v", err)
	}
	if err := release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	found := false
	for _, d := range ds {
		if d.Slug == "att" {
			found = true
		}
	}
	if !found {
		t.Fatal("domain not visible at its commit")
	}
}

func TestReader_UnknownRef(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	// A ref that is neither a live branch nor a valid revision fails cleanly.
	if _, _, err := app.Reader(context.Background(), ws, "definitely-not-a-branch"); err == nil {
		t.Fatal("expected an error reading an unknown ref")
	}
}

// --- Check ---------------------------------------------------------------

func TestCheck_DanglingRef(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	// Seed a spec, then a requirement whose statement references a nonexistent entity.
	mutate(t, ws, "main", func(ctx context.Context, w *app.Write) error {
		if _, e := store.AddDomain(ctx, w.Tx, store.Domain{Slug: "att", Name: "Att"}); e != nil {
			return e
		}
		w.MarkDirty("req_domain")
		if _, e := store.AddSpec(ctx, w.Tx, "att", store.Spec{Prefix: "ATT", Slug: "s", Title: "S"}); e != nil {
			return e
		}
		w.MarkDirty("req_spec")
		_, e := store.AddRequirement(ctx, w.Tx, "ATT", store.Requirement{Statement: "See [[REQ:GHOST-FR-999]] for details."})
		w.MarkDirty("req_requirement")
		return e
	})

	ctx := context.Background()
	r, release, err := app.Reader(ctx, ws, "main")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	findings, err := app.Check(ctx, r)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	var got *app.CheckFinding
	for i := range findings {
		if findings[i].Kind == "dangling-ref" && findings[i].Detail == "[[REQ:GHOST-FR-999]]" {
			got = &findings[i]
		}
	}
	if got == nil {
		t.Fatalf("expected a dangling-ref finding for [[REQ:GHOST-FR-999]], got %+v", findings)
	}
	if !strings.Contains(got.Location, "requirement") {
		t.Errorf("finding location should name the requirement: %q", got.Location)
	}
}

func TestCheck_Clean(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	seedGraph(t, ws)
	ctx := context.Background()
	r, release, err := app.Reader(ctx, ws, "main")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	findings, err := app.Check(ctx, r)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("clean graph should have no findings, got %+v", findings)
	}
}

// --- Impact / LabelIndex --------------------------------------------------

func TestImpact_EdgesAndTransitive(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	req1ID, req2ID := seedGraph(t, ws)

	// req1 --depends_on--> req2
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

	// Inbound to req2 = req1 via depends_on; transitive blast radius includes req1.
	rep, err := app.Impact(ctx, r, refs.Target{Type: "requirement", Key: "ATT-FR-002", ID: req2ID}, true)
	if err != nil {
		t.Fatalf("impact: %v", err)
	}
	if rep.Subject != "requirement:ATT-FR-002" {
		t.Errorf("subject = %q", rep.Subject)
	}
	if !hasLink(rep.Inbound, "requirement:ATT-FR-001", "depends_on") {
		t.Errorf("expected inbound requirement:ATT-FR-001 via depends_on, got %+v", rep.Inbound)
	}
	if !contains(rep.Transitive, "requirement:ATT-FR-001") {
		t.Errorf("expected requirement:ATT-FR-001 in transitive closure, got %v", rep.Transitive)
	}

	// Outbound from req1 = req2 via depends_on.
	rep1, err := app.Impact(ctx, r, refs.Target{Type: "requirement", Key: "ATT-FR-001", ID: req1ID}, false)
	if err != nil {
		t.Fatalf("impact req1: %v", err)
	}
	if !hasLink(rep1.Outbound, "requirement:ATT-FR-002", "depends_on") {
		t.Errorf("expected outbound requirement:ATT-FR-002 via depends_on, got %+v", rep1.Outbound)
	}
}

func TestLabelIndex(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	req1ID, _ := seedGraph(t, ws)
	ctx := context.Background()
	r, release, err := app.Reader(ctx, ws, "main")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	label, err := app.LabelIndex(ctx, r)
	if err != nil {
		t.Fatalf("label index: %v", err)
	}
	if got := label("requirement", req1ID); got != "requirement:ATT-FR-001" {
		t.Errorf("label(req1) = %q, want requirement:ATT-FR-001", got)
	}
	if got := label("entity", "no-such-id"); got != "entity:no-such-id" {
		t.Errorf("unknown id should fall back to type:id, got %q", got)
	}
}

// --- refs: LoadResolver / IngestRefs / ReconcileRefs ----------------------

func TestIngestAndReconcileRefs(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	req1ID, _ := seedGraph(t, ws)

	// Ingest a reference from req1 to req2, then persist the resolved entity_ref rows.
	mutate(t, ws, "main", func(ctx context.Context, w *app.Write) error {
		resolver, e := app.LoadResolver(ctx, w.Tx)
		if e != nil {
			return e
		}
		_, res := app.IngestRefs(resolver, "requirement", req1ID, "Depends on [[REQ:ATT-FR-002]].")
		if len(res.Targets) != 1 {
			t.Fatalf("expected one ingested target, got %+v", res.Targets)
		}
		if err := app.DanglingError(res.Dangling); err != nil {
			return err
		}
		return app.ReconcileRefs(ctx, w, "requirement", req1ID, res.Targets)
	})

	// The persisted ref shows up as an outbound "ref" link from req1.
	ctx := context.Background()
	r, release, err := app.Reader(ctx, ws, "main")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	rep, err := app.Impact(ctx, r, refs.Target{Type: "requirement", Key: "ATT-FR-001", ID: req1ID}, false)
	if err != nil {
		t.Fatalf("impact: %v", err)
	}
	if !hasLink(rep.Outbound, "requirement:ATT-FR-002", "ref") {
		t.Errorf("expected outbound ref to requirement:ATT-FR-002, got %+v", rep.Outbound)
	}
}

func TestResolveRef_AgainstDB(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	seedGraph(t, ws)
	ctx := context.Background()
	r, release, err := app.Reader(ctx, ws, "main")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	resolver, err := app.LoadResolver(ctx, r)
	if err != nil {
		t.Fatalf("load resolver: %v", err)
	}
	tg, ok := app.ResolveRef(resolver, "ATT-FR-001")
	if !ok || tg.Type != "requirement" {
		t.Fatalf("ResolveRef bare fr_key: ok=%v target=%+v", ok, tg)
	}
	if tg2, ok := app.ResolveRef(resolver, "SPEC:ATT"); !ok || tg2.Type != "spec" {
		t.Fatalf("ResolveRef SPEC:ATT: ok=%v target=%+v", ok, tg2)
	}
	if _, ok := app.ResolveRef(resolver, "REQ:MISSING-FR-1"); ok {
		t.Fatal("ResolveRef of a missing key should not resolve")
	}
}

// --- Export ---------------------------------------------------------------

func TestExport_Deterministic(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	seedGraph(t, ws)

	dump := func() (app.ExportStats, string) {
		ctx := context.Background()
		r, release, err := app.Reader(ctx, ws, "main")
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		var buf bytes.Buffer
		stats, err := app.Export(ctx, r, &buf)
		if err != nil {
			t.Fatalf("export: %v", err)
		}
		return stats, buf.String()
	}

	stats1, out1 := dump()
	_, out2 := dump()
	if out1 != out2 {
		t.Fatal("export is not byte-for-byte deterministic across runs")
	}
	if stats1.Tables == 0 || stats1.Rows == 0 {
		t.Fatalf("expected a non-empty export, got %+v", stats1)
	}
	// The migration cursor table is excluded.
	if strings.Contains(out1, `"schema_migrations"`) {
		t.Error("schema_migrations should be excluded from the export")
	}
	// Every emitted line is a valid JSONL record naming a table.
	lines := 0
	for _, line := range strings.Split(strings.TrimSpace(out1), "\n") {
		if line == "" {
			continue
		}
		var rec struct {
			Table string         `json:"table"`
			Row   map[string]any `json:"row"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("invalid JSONL line %q: %v", line, err)
		}
		if rec.Table == "" {
			t.Errorf("line missing table name: %q", line)
		}
		lines++
	}
	if lines != stats1.Rows {
		t.Errorf("line count %d != reported rows %d", lines, stats1.Rows)
	}
}

// --- GatherPrime ----------------------------------------------------------

func TestGatherPrime(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	seedGraph(t, ws)

	st := app.GatherPrime(context.Background(), ws)
	if st == nil {
		t.Fatal("nil prime state")
	}
	if st.ActiveChangeset != "" {
		t.Errorf("expected no active changeset, got %q", st.ActiveChangeset)
	}
	if st.CheckFindings != 0 {
		t.Errorf("clean graph should report 0 check findings, got %d", st.CheckFindings)
	}
	byKind := map[string]int{}
	for _, s := range st.Stats {
		byKind[s.Kind] = s.Count
	}
	if byKind["domains"] < 1 || byKind["specs"] < 1 || byKind["requirements"] < 2 {
		t.Errorf("headline stats look wrong: %+v", st.Stats)
	}
}

// --- EntityDiffs ----------------------------------------------------------

func TestEntityDiffs_ModifiedSpec(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	seedGraph(t, ws) // creates spec ATT with title "Take Attendance" on main

	ctx := context.Background()
	if err := app.CreateBranch(ctx, ws, "changeset/diff"); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	// Edit the spec title on the changeset branch.
	mutate(t, ws, "changeset/diff", func(ctx context.Context, w *app.Write) error {
		sp, ok, e := store.GetSpec(ctx, w.Tx, "ATT")
		if e != nil || !ok {
			t.Fatalf("get spec: ok=%v err=%v", ok, e)
		}
		if e := store.UpdateSpec(ctx, w.Tx, sp.ID, "Record Attendance", sp.Status); e != nil {
			return e
		}
		w.MarkDirty("req_spec")
		return nil
	})

	// Diff base (main) vs head (changeset/diff) on a pinned connection anchored to main.
	conn, err := ws.Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := versioncontrolops.CheckoutBranch(ctx, conn, "main"); err != nil {
		t.Fatal(err)
	}
	diffs, err := app.EntityDiffs(ctx, conn, "main", "changeset/diff")
	if err != nil {
		t.Fatalf("entity diffs: %v", err)
	}
	var specDiff *app.EntityDiff
	for i := range diffs {
		if diffs[i].SubjectType == "spec" && diffs[i].ChangeType == "modified" {
			specDiff = &diffs[i]
		}
	}
	if specDiff == nil {
		t.Fatalf("expected a modified spec diff, got %+v", diffs)
	}
	if specDiff.DocRef != "spec:ATT" {
		t.Errorf("spec DocRef = %q, want spec:ATT", specDiff.DocRef)
	}
	var titleField *app.FieldDiff
	for i := range specDiff.Fields {
		if specDiff.Fields[i].Name == "title" {
			titleField = &specDiff.Fields[i]
		}
	}
	if titleField == nil {
		t.Fatalf("expected a title field diff, got %+v", specDiff.Fields)
	}
	if titleField.Base != "Take Attendance" || titleField.Head != "Record Attendance" {
		t.Errorf("title diff = %q -> %q", titleField.Base, titleField.Head)
	}
}

// --- Branch operations ----------------------------------------------------

func TestBranchOperations(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	ctx := context.Background()

	bl, err := app.Branches(ctx, ws)
	if err != nil {
		t.Fatalf("branches: %v", err)
	}
	if bl.Active != "main" || !contains(bl.Branches, "main") {
		t.Fatalf("initial branch list wrong: %+v", bl)
	}

	if err := app.CreateBranch(ctx, ws, "feature-x"); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	if bl2, _ := app.Branches(ctx, ws); !contains(bl2.Branches, "feature-x") {
		t.Fatalf("feature-x not listed: %+v", bl2)
	}

	// Checkout makes it the active read/write target.
	if err := app.Checkout(ctx, ws, "feature-x"); err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if got := app.ResolveBranch(ws, ""); got != "feature-x" {
		t.Errorf("active target after checkout = %q", got)
	}
	if bl3, _ := app.Branches(ctx, ws); bl3.Active != "feature-x" {
		t.Errorf("Branches.Active = %q, want feature-x", bl3.Active)
	}

	// Checkout main clears the pointer.
	if err := app.Checkout(ctx, ws, "main"); err != nil {
		t.Fatalf("checkout main: %v", err)
	}
	if got := app.ResolveBranch(ws, ""); got != "main" {
		t.Errorf("active target after checkout main = %q", got)
	}

	// Checkout of a nonexistent branch is a not-found error.
	if err := app.Checkout(ctx, ws, "no-such-branch"); err == nil {
		t.Error("expected not-found for unknown branch checkout")
	}

	// Delete works once it is not the active target.
	if err := app.DeleteBranch(ctx, ws, "feature-x"); err != nil {
		t.Fatalf("delete branch: %v", err)
	}
	if bl4, _ := app.Branches(ctx, ws); contains(bl4.Branches, "feature-x") {
		t.Fatalf("feature-x still listed after delete: %+v", bl4)
	}
}

// --- Maintenance / SyncConfiguredFull -------------------------------------

func TestMaintenanceReads(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	addDomain(t, ws, "main", "att") // ensure >1 commit

	ctx := context.Background()
	count, hash, err := app.FlattenPreview(ctx, ws)
	if err != nil {
		t.Fatalf("flatten preview: %v", err)
	}
	if count < 2 {
		t.Errorf("expected >= 2 commits, got %d", count)
	}
	if hash == "" {
		t.Error("flatten preview returned empty root hash")
	}

	// Dry-run compaction with a normal cutoff: all commits are recent, nothing compacted.
	res, err := app.CompactDolt(ctx, ws, 30, true, time.Now())
	if err != nil {
		t.Fatalf("compact dry run: %v", err)
	}
	if res.Compacted {
		t.Error("dry run should not compact")
	}
	if res.CutoffDays != 30 || res.TotalCommits < 2 {
		t.Errorf("compact result: %+v", res)
	}

	// Dry-run with a cutoff far in the future: all commits fall before it (old).
	future := time.Now().AddDate(1, 0, 0)
	resOld, err := app.CompactDolt(ctx, ws, 30, true, future)
	if err != nil {
		t.Fatalf("compact (all old) dry run: %v", err)
	}
	if resOld.Compacted {
		t.Error("dry run should not compact even when all commits are old")
	}
	if resOld.OldCommits < 1 || resOld.BoundaryHash == "" {
		t.Errorf("expected old commits and a boundary hash, got %+v", resOld)
	}
}

func TestSyncConfiguredFull_NoFormats(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	// The default config has no formats, so a full sync is a no-op with empty stats.
	stats, err := app.SyncConfiguredFull(context.Background(), ws)
	if err != nil {
		t.Fatalf("sync configured full: %v", err)
	}
	if stats == nil {
		t.Fatal("expected non-nil stats")
	}
	if stats.Written != 0 || stats.Removed != 0 {
		t.Errorf("expected an empty sync, got %+v", stats)
	}
}

// --- helpers --------------------------------------------------------------

func hasLink(ls []app.ImpactLink, endpoint, via string) bool {
	for _, l := range ls {
		if l.Endpoint == endpoint && l.Via == via {
			return true
		}
	}
	return false
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
