package generate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/endermalkoc/cusp/internal/refs"
)

// richModel builds a fully-populated Model by hand (no store / Dolt) so the pure renderers
// (Markdown, JSON, HTML, planning) can be exercised in isolation. It mirrors what Load would
// assemble: two domains, two specs (one richly-sectioned with stories/scenarios/groups/FRs),
// two entities, glossary terms, a planning layer, and a navigation tree + ref targets.
func richModel() *Model {
	targets := []refs.Target{
		{Type: "TERM", Key: "student", ID: "t1", DocPath: "glossary.md", Anchor: "student", Label: "Student"},
	}
	m := &Model{
		Domains: []*Domain{
			{Slug: "enrollment", Name: "Enrollment", Description: "Signing students up", Status: "active"},
			{Slug: "billing", Name: "Billing", Description: "", Status: "draft"},
		},
		Specs: []*Spec{
			{
				Prefix: "ENR", Slug: "signup", Title: "Signup Flow", Domain: "enrollment",
				Status: "active", Created: "2026-01-02", Path: "enrollment/signup.md",
				Sections: []*Section{
					{Key: "preamble", Title: "", Level: 0, Position: 5, Body: "Intro prose."},
					{Key: "overview", Title: "Overview", Level: 2, Position: 10, Body: "See [[TERM:student]] for the actor."},
					{Key: "edge_cases", Title: "Edge Cases", Level: 3, Position: 20, Body: "What if the email is taken?"},
					{Key: "more_info", Title: "", Level: 0, Position: 30, Body: "Trailing FR-area prose."},
					{Key: "success_criteria", Title: "Success Criteria", Level: 2, Position: 40, Body: "All students can sign up."},
				},
				Stories: []*Story{
					{
						Position: 1, Title: "Self serve signup", Priority: 1, AsA: "student",
						IWant: "to register", SoThat: "I can enroll", WhyPriority: "core flow",
						IndependentTest: "register a fresh account",
						Scenarios: []*Scenario{
							{Position: 1, Given: "a new email", When: "I submit", Then: "an account exists"},
							{Position: 2, Given: "a duplicate email", Then: "I see an error"},
						},
					},
					{
						Position: 2, Title: "Narrative only", Priority: 9, Narrative: "As an admin I can invite students.",
					},
				},
				Groups: []*Group{
					{ID: "g1", Position: 1, Title: "Account", Notes: "Group note."},
				},
				Requirements: []*Requirement{
					{FRKey: "ENR-FR-001", Number: 1, GroupID: "g1", Position: 1, Statement: "The system MUST accept an email."},
					{FRKey: "ENR-FR-002", Number: 2, GroupID: "g1", Position: 2, Statement: ""},
					{FRKey: "ENR-FR-003", Number: 3, GroupID: "", Position: 3, Statement: "Ungrouped requirement."},
				},
			},
			{
				Slug: "invoice", Title: "", Domain: "billing", Status: "draft", Path: "billing/invoice.md",
			},
		},
		Entities: []*Entity{
			{Name: "Student", Description: "A learner.", Status: "active", DocPath: "entities/student.md",
				Sections: []*Section{
					{Key: "purpose", Title: "Purpose", Level: 2, Position: 10, Body: "Represents a learner."},
				}},
			{Name: "Course", Description: "A unit of study.", Status: "draft", DocPath: "entities/course.md"},
		},
		Terms: []*Term{
			{Slug: "student", Term: "Student", Definition: "A person enrolled in a course.", DomainSlug: "enrollment", Aliases: []string{"learner", "pupil"}},
			{Slug: "invoice", Definition: "", DomainSlug: ""},
		},
		Capabilities: []*Capability{
			{ID: "cap-root", Title: "Enroll Students", Level: "epic", DomainSlug: "enrollment", Milestones: []string{"M0", "M1"}, Deliverables: []string{"Signup form"}, NotionURL: "https://notion/cap-root"},
			{ID: "cap-child", Title: "Bulk Import", Level: "capability", DomainSlug: "enrollment", ParentID: "cap-root", ParentTitle: "Enroll Students"},
			{ID: "cap-orphan", Title: "Reporting", Level: "capability"},
		},
		Deliverables: []*Deliverable{
			{ID: "d1", Title: "Signup form", Size: "M", Status: "built", AIReady: "yes", Milestone: "M0", Capabilities: []string{"Enroll Students"}, Views: []string{"Signup Page"}, BeadIDs: "bd-1"},
			{ID: "d2", Title: "Importer", Size: "L", Status: "proposed", AIReady: "no", Milestone: "", BlockedBy: []string{"Signup form"}},
		},
		Views: []*View{
			{ID: "v1", Title: "Signup Page", Route: "/signup", DomainSlug: "enrollment", SpecPath: "enrollment/signup.md", SpecTitle: "Signup Flow", Deliverables: []string{"Signup form"}},
			{ID: "v2", Title: "Dashboard", Route: "", DomainSlug: ""},
		},
		Targets:    targets,
		Priorities: map[int]string{1: "High", 2: "Medium"},
	}
	m.Nav = navFor(m)
	return m
}

// navFor builds a Nav tree consistent with the model, mirroring what loadNav produces.
func navFor(m *Model) *Nav {
	nav := &Nav{DirLabel: map[string]string{}}
	specsRoot := &NavNode{Label: "Specifications", Href: "index.html", Kind: "specs"}
	nav.Root = append(nav.Root, specsRoot)
	for _, d := range m.Domains {
		nav.DirLabel[d.Slug] = d.Name
		specsRoot.Children = append(specsRoot.Children, &NavNode{
			Label: d.Name, Href: d.Slug + "/index.html", Seg: d.Slug, Kind: "domain",
			Children: []*NavNode{{Label: "leaf", Href: d.Slug + "/leaf.html", Seg: "leaf", Kind: "spec"}},
		})
	}
	nav.DirLabel["planning"] = "Planning"
	nav.Root = append(nav.Root, &NavNode{Label: "Planning", Href: "planning/index.html", Seg: "planning", Kind: "planning",
		Children: []*NavNode{{Label: "Capabilities", Href: "planning/capabilities.html", Seg: "capabilities", Kind: "capability"}}})
	nav.DirLabel["entities"] = "Entities"
	nav.Root = append(nav.Root, &NavNode{Label: "Entities", Href: "entities/index.html", Seg: "entities", Kind: "entities"})
	nav.Root = append(nav.Root, &NavNode{Label: "Glossary", Href: "glossary.html", Seg: "glossary", Kind: "glossary"})
	return nav
}

// filesByPath renders and indexes files by their path for assertions.
func filesByPath(t *testing.T, files []File) map[string]File {
	t.Helper()
	out := map[string]File{}
	for _, f := range files {
		if _, dup := out[f.Path]; dup {
			t.Fatalf("duplicate rendered path %q", f.Path)
		}
		out[f.Path] = f
	}
	return out
}

// ---- Markdown renderer -----------------------------------------------------

func TestMarkdownRender(t *testing.T) {
	m := richModel()
	files, err := newMarkdownRenderer(m).Render(m)
	if err != nil {
		t.Fatalf("markdown render: %v", err)
	}
	byPath := filesByPath(t, files)

	for _, want := range []string{
		"enrollment/signup.md", "billing/invoice.md",
		"index.md", "enrollment/index.md", "billing/index.md",
		"entities/student.md", "entities/course.md", "entities/index.md",
		"glossary.md",
		"planning/index.md", "planning/capabilities.md", "planning/deliverables.md", "planning/views.md",
	} {
		if _, ok := byPath[want]; !ok {
			t.Errorf("missing markdown file %q", want)
		}
	}

	spec := byPath["enrollment/signup.md"].Content
	for _, sub := range []string{
		"id: ENR", "title: Signup Flow", "domain: enrollment", "status: Active", "created: 2026-01-02",
		"# Signup Flow",
		"## User Scenarios & Testing",
		"### User Story 1 - Self serve signup (Priority: 1 - High)",
		"As a student, I want to register so that I can enroll.",
		"**Why this priority**: core flow",
		"**Independent Test**: register a fresh account",
		"1. **Given** a new email, **When** I submit, **Then** an account exists",
		"2. **Given** a duplicate email, **Then** I see an error", // no When → two-clause form
		"As an admin I can invite students.",                      // narrative-only story
		"### Edge Cases",
		"## Requirements",
		"**Account**",
		"Group note.",
		"- **ENR-FR-001**: The system MUST accept an email. ^enr-fr-001",
		"- **ENR-FR-002**: ^enr-fr-002", // empty statement branch
		"Trailing FR-area prose.",       // more_info after FR list
		"## Success Criteria",
	} {
		if !strings.Contains(spec, sub) {
			t.Errorf("spec markdown missing %q\n---\n%s", sub, spec)
		}
	}

	// Entity Description→Purpose fallback: Course has no purpose section, so its description prints.
	course := byPath["entities/course.md"].Content
	if !strings.Contains(course, "# Course") || !strings.Contains(course, "A unit of study.") {
		t.Errorf("entity markdown missing description fallback:\n%s", course)
	}
	// Student has a purpose section, so the description is NOT repeated as a bare block.
	student := byPath["entities/student.md"].Content
	if !strings.Contains(student, "Represents a learner.") {
		t.Errorf("entity purpose section missing:\n%s", student)
	}

	gloss := byPath["glossary.md"].Content
	for _, sub := range []string{"# Glossary", "## Student", "aka learner, pupil", "domain: enrollment", "^student", "A person enrolled in a course."} {
		if !strings.Contains(gloss, sub) {
			t.Errorf("glossary markdown missing %q", sub)
		}
	}

	// Domain index links + "See also" carries Planning because the model has planning data.
	idx := byPath["index.md"].Content
	if !strings.Contains(idx, "[[planning/index|Planning]]") || !strings.Contains(idx, "[[entities/index|Entities]]") {
		t.Errorf("index See-also wrong:\n%s", idx)
	}

	// Billing domain page: title falls back to name, empty description omitted, spec listed.
	billing := byPath["billing/index.md"].Content
	if !strings.Contains(billing, "# Billing") || !strings.Contains(billing, "[[billing/invoice|invoice]]") {
		t.Errorf("billing domain page wrong:\n%s", billing)
	}

	caps := byPath["planning/capabilities.md"].Content
	for _, sub := range []string{"# Capabilities", "**Enroll Students**", "`epic`", "M0, M1", "1 deliverable", "**Bulk Import**", "**Reporting**"} {
		if !strings.Contains(caps, sub) {
			t.Errorf("capabilities markdown missing %q\n%s", sub, caps)
		}
	}
	delivs := byPath["planning/deliverables.md"].Content
	for _, sub := range []string{"# Deliverables", "## M0", "## Unscheduled", "Signup form", "Yes", "No"} {
		if !strings.Contains(delivs, sub) {
			t.Errorf("deliverables markdown missing %q\n%s", sub, delivs)
		}
	}
	views := byPath["planning/views.md"].Content
	for _, sub := range []string{"# Views", "Signup Page", "`/signup`", "[[enrollment/signup\\|Signup Flow]]", "## Unassigned"} {
		if !strings.Contains(views, sub) {
			t.Errorf("views markdown missing %q\n%s", sub, views)
		}
	}
}

func TestMarkdownRender_NoPlanningNoGlossary(t *testing.T) {
	m := &Model{
		Domains: []*Domain{{Slug: "d", Name: "D", Status: "active"}},
		Specs:   []*Spec{{Slug: "s", Domain: "d", Status: "draft", Path: "d/s.md"}},
	}
	m.Nav = &Nav{DirLabel: map[string]string{}}
	files, err := newMarkdownRenderer(m).Render(m)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	byPath := filesByPath(t, files)
	if _, ok := byPath["glossary.md"]; ok {
		t.Error("glossary.md should not be rendered without terms")
	}
	if _, ok := byPath["planning/index.md"]; ok {
		t.Error("planning pages should not be rendered without planning data")
	}
	// Domain with no specs shows the empty placeholder.
	empty := &Model{Domains: []*Domain{{Slug: "x", Name: "X", Status: "active"}}}
	f2, _ := newMarkdownRenderer(empty).Render(empty)
	bp := filesByPath(t, f2)
	if !strings.Contains(bp["x/index.md"].Content, "_No specs yet._") {
		t.Errorf("empty domain page missing placeholder: %s", bp["x/index.md"].Content)
	}
	// index See-also omits Planning when there is none.
	if strings.Contains(bp["index.md"].Content, "Planning") {
		t.Errorf("index should not mention Planning: %s", bp["index.md"].Content)
	}
}

// ---- JSON renderer ---------------------------------------------------------

func TestJSONRender(t *testing.T) {
	m := richModel()
	files, err := jsonRenderer{}.Render(m)
	if err != nil {
		t.Fatalf("json render: %v", err)
	}
	byPath := filesByPath(t, files)
	for _, want := range []string{
		"enrollment/signup.json", "billing/invoice.json",
		"entities/student.json", "entities/course.json",
		"index.json", "glossary.json", "planning.json",
	} {
		if _, ok := byPath[want]; !ok {
			t.Errorf("missing json file %q", want)
		}
	}

	// index.json parses into a Manifest with the expected discovery refs.
	var man Manifest
	if err := json.Unmarshal([]byte(byPath["index.json"].Content), &man); err != nil {
		t.Fatalf("index.json parse: %v", err)
	}
	if !man.Glossary || !man.Planning {
		t.Errorf("manifest flags wrong: glossary=%v planning=%v", man.Glossary, man.Planning)
	}
	if len(man.Domains) != 2 || len(man.Specs) != 2 || len(man.Entities) != 2 {
		t.Fatalf("manifest counts: domains=%d specs=%d entities=%d", len(man.Domains), len(man.Specs), len(man.Entities))
	}
	var enr *ManifestRef
	for i := range man.Specs {
		if man.Specs[i].Path == "enrollment/signup.json" {
			enr = &man.Specs[i]
		}
	}
	if enr == nil || enr.Key != "ENR" || enr.Title != "Signup Flow" || enr.Domain != "enrollment" {
		t.Fatalf("enrollment manifest ref wrong: %+v", enr)
	}
	// The prefix-less billing spec keys on its slug.
	var bill *ManifestRef
	for i := range man.Specs {
		if man.Specs[i].Path == "billing/invoice.json" {
			bill = &man.Specs[i]
		}
	}
	if bill == nil || bill.Key != "invoice" {
		t.Fatalf("billing manifest ref should key on slug: %+v", bill)
	}

	// A spec JSON keeps inline ref tokens verbatim (data family does not resolve them).
	spec := byPath["enrollment/signup.json"].Content
	if !strings.Contains(spec, "[[TERM:student]]") {
		t.Errorf("spec json should keep raw ref token:\n%s", spec)
	}
	if !strings.Contains(spec, `"fr_key": "ENR-FR-001"`) {
		t.Errorf("spec json missing requirement: %s", spec)
	}
}

func TestJSONRender_MinimalOmitsRollups(t *testing.T) {
	m := &Model{Domains: []*Domain{{Slug: "d", Name: "D", Status: "active"}}}
	files, err := jsonRenderer{}.Render(m)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	byPath := filesByPath(t, files)
	if _, ok := byPath["glossary.json"]; ok {
		t.Error("glossary.json should be omitted with no terms")
	}
	if _, ok := byPath["planning.json"]; ok {
		t.Error("planning.json should be omitted with no planning data")
	}
	var man Manifest
	if err := json.Unmarshal([]byte(byPath["index.json"].Content), &man); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if man.Glossary || man.Planning {
		t.Errorf("flags should be false: %+v", man)
	}
}

func TestJSONPath(t *testing.T) {
	cases := map[string]string{
		"enrollment/signup.md": "enrollment/signup.json",
		"entities/x.md":        "entities/x.json",
		"noext":                "noext.json",
	}
	for in, want := range cases {
		if got := jsonPath(in); got != want {
			t.Errorf("jsonPath(%q)=%q want %q", in, got, want)
		}
	}
}

// ---- HTML renderer ---------------------------------------------------------

func TestHTMLRender(t *testing.T) {
	m := richModel()
	files, err := newHTMLRenderer(m).Render(m)
	if err != nil {
		t.Fatalf("html render: %v", err)
	}
	byPath := filesByPath(t, files)
	for _, want := range []string{
		"enrollment/signup.html", "billing/invoice.html",
		"index.html", "enrollment/index.html",
		"entities/student.html", "entities/index.html", "glossary.html",
		"planning/index.html", "planning/capabilities.html", "planning/deliverables.html", "planning/views.html",
		assetStylePath,
	} {
		if _, ok := byPath[want]; !ok {
			t.Errorf("missing html file %q", want)
		}
	}
	if byPath[assetStylePath].Kind != "asset" {
		t.Errorf("stylesheet kind = %q, want asset", byPath[assetStylePath].Kind)
	}
	if !strings.Contains(byPath[assetStylePath].Content, ".nav-tree") {
		t.Errorf("stylesheet content looks wrong")
	}

	spec := byPath["enrollment/signup.html"].Content
	for _, sub := range []string{
		"<!DOCTYPE html>", "Signup Flow · Cusp",
		`<link rel="stylesheet"`, `class="nav-tree"`, `class="breadcrumbs"`,
		`class="prio prio-1"`,     // priority badge from decoratePriorities
		`<div class="doc-meta">`,  // meta bar injected under H1
		`class="meta-chip status`, // status chip
		`<a class="xref"`,         // inline ref styled as xref
	} {
		if !strings.Contains(spec, sub) {
			t.Errorf("spec html missing %q", sub)
		}
	}

	// Index page renders the browsable content tree.
	index := byPath["index.html"].Content
	if !strings.Contains(index, `class="content-tree"`) || !strings.Contains(index, `class="lede"`) {
		t.Errorf("index html missing content tree")
	}

	// Planning HTML comes straight from the Model (pills, cap tree, cards).
	caps := byPath["planning/capabilities.html"].Content
	for _, sub := range []string{`class="cap-tree"`, `class="pill pill-level`, "Enroll Students", "Bulk Import"} {
		if !strings.Contains(caps, sub) {
			t.Errorf("capabilities html missing %q", sub)
		}
	}
	delivs := byPath["planning/deliverables.html"].Content
	if !strings.Contains(delivs, "AI-ready") || !strings.Contains(delivs, "Not AI-ready") {
		t.Errorf("deliverables html missing ai pills")
	}
	planIndex := byPath["planning/index.html"].Content
	if !strings.Contains(planIndex, `class="plan-card"`) {
		t.Errorf("planning index missing cards")
	}
	viewsHTML := byPath["planning/views.html"].Content
	if !strings.Contains(viewsHTML, `<code>/signup</code>`) || !strings.Contains(viewsHTML, `class="xref"`) {
		t.Errorf("views html missing route/spec link")
	}
}

// ---- render_one (single-doc chokepoint), pure over a Model ------------------

func TestRenderLoadedDoc(t *testing.T) {
	m := richModel()

	md, err := renderLoadedDoc(m, "enrollment/signup.md", "md")
	if err != nil {
		t.Fatalf("md: %v", err)
	}
	if !strings.Contains(md, "# Signup Flow") || strings.Contains(md, "<article") {
		t.Errorf("md output wrong:\n%s", md)
	}

	page, err := renderLoadedDoc(m, "enrollment/signup.md", "html")
	if err != nil {
		t.Fatalf("html: %v", err)
	}
	for _, sub := range []string{"<!DOCTYPE html>", `class="embedded"`, "<style>", "Signup Flow · Cusp"} {
		if !strings.Contains(page, sub) {
			t.Errorf("embedded html missing %q", sub)
		}
	}
	// Embedded page has no site chrome (sidebar / breadcrumbs).
	if strings.Contains(page, `class="nav-tree"`) || strings.Contains(page, `class="breadcrumbs"`) {
		t.Errorf("embedded page should not carry site chrome")
	}

	if _, err := renderLoadedDoc(m, "nope/missing.md", "md"); err == nil {
		t.Error("expected error for missing doc path")
	}
	if _, err := renderLoadedDoc(m, "enrollment/signup.md", "pdf"); err == nil {
		t.Error("expected error for unknown format")
	}
}

// ---- rendererFor -----------------------------------------------------------

func TestRendererFor(t *testing.T) {
	m := &Model{}
	for _, f := range []string{"", "md", "markdown", "MD"} {
		r, err := rendererFor(f, m)
		if err != nil {
			t.Fatalf("rendererFor(%q): %v", f, err)
		}
		if _, ok := r.(*markdownRenderer); !ok {
			t.Errorf("rendererFor(%q) not markdown: %T", f, r)
		}
	}
	if r, err := rendererFor("json", m); err != nil || func() bool { _, ok := r.(jsonRenderer); return !ok }() {
		t.Errorf("rendererFor(json) = %T, %v", r, err)
	}
	if r, err := rendererFor("HTML", m); err != nil || r == nil {
		t.Errorf("rendererFor(HTML) = %T, %v", r, err)
	}
	if _, err := rendererFor("xml", m); err == nil {
		t.Error("expected error for unknown format")
	}
}

// ---- canonicalFormat / Stats / DirtySet ------------------------------------

func TestCanonicalFormat(t *testing.T) {
	cases := map[string]string{
		"": "markdown", "md": "markdown", "markdown": "markdown", "MD": "markdown",
		"json": "json", "JSON": "json", "html": "html", "HTML": "html",
		"weird": "weird",
	}
	for in, want := range cases {
		if got := canonicalFormat(in); got != want {
			t.Errorf("canonicalFormat(%q)=%q want %q", in, got, want)
		}
	}
}

func TestStatsTotal(t *testing.T) {
	s := Stats{Specs: 2, Entities: 3, Indexes: 4, Glossary: 1, Planning: 2}
	if got := s.Total(); got != 12 {
		t.Errorf("Total()=%d want 12", got)
	}
	if (Stats{}).Total() != 0 {
		t.Error("empty Total should be 0")
	}
}

func TestDirtySetEmpty(t *testing.T) {
	if !(DirtySet{}).empty() {
		t.Error("zero DirtySet should be empty")
	}
	if (DirtySet{Full: true}).empty() {
		t.Error("Full set is not empty")
	}
	if (DirtySet{SpecIDs: []string{"a"}}).empty() {
		t.Error("set with spec ids is not empty")
	}
	if (DirtySet{EntityIDs: []string{"e"}}).empty() {
		t.Error("set with entity ids is not empty")
	}
}

// ---- keepDocs --------------------------------------------------------------

func TestKeepDocs(t *testing.T) {
	in := []File{
		{Path: "a", Kind: "spec"},
		{Path: "b", Kind: "entity"},
		{Path: "c", Kind: "index"},
		{Path: "d", Kind: "glossary"},
		{Path: "e", Kind: "planning"},
		{Path: "f", Kind: "asset"},
	}
	got := keepDocs(in)
	var kinds []string
	for _, f := range got {
		kinds = append(kinds, f.Kind)
	}
	want := []string{"spec", "entity", "asset"}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Errorf("keepDocs kept %v, want %v", kinds, want)
	}
}

// ---- humanizeSegment / Nav helpers -----------------------------------------

func TestHumanizeSegment(t *testing.T) {
	cases := map[string]string{
		"student-detail": "Student Detail",
		"snake_case_seg": "Snake Case Seg",
		"single":         "Single",
		"":               "",
	}
	for in, want := range cases {
		if got := humanizeSegment(in); got != want {
			t.Errorf("humanizeSegment(%q)=%q want %q", in, got, want)
		}
	}
}

func TestNavHelpers(t *testing.T) {
	nav := &Nav{DirLabel: map[string]string{"enrollment": "Enrollment", "entities": "Entities"}}
	if !nav.HasIndex("enrollment") {
		t.Error("enrollment should have an index")
	}
	if nav.HasIndex("enrollment/student-detail") {
		t.Error("sub-directory should not report an index")
	}
	if got := nav.SegLabel("enrollment", "enrollment"); got != "Enrollment" {
		t.Errorf("SegLabel domain=%q want Enrollment", got)
	}
	if got := nav.SegLabel("enrollment/student-detail", "student-detail"); got != "Student Detail" {
		t.Errorf("SegLabel humanized=%q want Student Detail", got)
	}
}

// ---- hashContent -----------------------------------------------------------

func TestHashContent(t *testing.T) {
	// SHA-256 of "abc" (known vector).
	if got := hashContent("abc"); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Errorf("hashContent(abc)=%q", got)
	}
	if hashContent("x") == hashContent("y") {
		t.Error("distinct inputs must hash differently")
	}
	if hashContent("same") != hashContent("same") {
		t.Error("hash must be deterministic")
	}
}

// ---- manifest + reconcile (filesystem, no Dolt) ----------------------------

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	// Missing manifest → empty map, no error.
	got, err := loadManifest(dir)
	if err != nil {
		t.Fatalf("loadManifest missing: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("missing manifest should be empty, got %v", got)
	}
	want := map[string]string{"a.md": "h1", "b.md": "h2"}
	if err := saveManifest(dir, want); err != nil {
		t.Fatalf("saveManifest: %v", err)
	}
	got, err = loadManifest(dir)
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	if len(got) != 2 || got["a.md"] != "h1" || got["b.md"] != "h2" {
		t.Errorf("round-trip mismatch: %v", got)
	}
	// A corrupt manifest degrades to an empty map (never fails the mutation).
	if err := os.WriteFile(manifestPath(dir), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = loadManifest(dir)
	if err != nil {
		t.Fatalf("corrupt manifest should not error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("corrupt manifest should read as empty, got %v", got)
	}
}

func TestReconcile(t *testing.T) {
	dir := t.TempDir()
	files := []File{
		{Path: "a.md", Content: "alpha", Kind: "spec"},
		{Path: "sub/b.md", Content: "beta", Kind: "entity"},
	}
	// First full reconcile writes both and records the manifest.
	written, removed, err := reconcile(dir, files, true)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if written != 2 || removed != 0 {
		t.Fatalf("first reconcile: written=%d removed=%d", written, removed)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "a.md")); string(b) != "alpha\n" {
		t.Errorf("a.md content = %q", b)
	}

	// Re-running with identical content writes nothing (hash match).
	written, removed, err = reconcile(dir, files, true)
	if err != nil {
		t.Fatalf("reconcile 2: %v", err)
	}
	if written != 0 || removed != 0 {
		t.Fatalf("idempotent reconcile: written=%d removed=%d", written, removed)
	}

	// Changing one file rewrites only it.
	files[0].Content = "alpha-2"
	written, _, err = reconcile(dir, files, true)
	if err != nil {
		t.Fatalf("reconcile 3: %v", err)
	}
	if written != 1 {
		t.Fatalf("changed reconcile: written=%d want 1", written)
	}

	// Dropping a file with removeOrphans deletes it and records the removal.
	written, removed, err = reconcile(dir, files[:1], true)
	if err != nil {
		t.Fatalf("reconcile 4: %v", err)
	}
	if removed != 1 {
		t.Fatalf("orphan removal: removed=%d want 1", removed)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub/b.md")); !os.IsNotExist(err) {
		t.Errorf("orphan file should be gone, stat err=%v", err)
	}

	// Fast path (removeOrphans=false) keeps prior manifest entries and never deletes.
	dir2 := t.TempDir()
	if _, _, err := reconcile(dir2, files, true); err != nil {
		t.Fatal(err)
	}
	written, removed, err = reconcile(dir2, files[:1], false)
	if err != nil {
		t.Fatalf("reconcile fast: %v", err)
	}
	if removed != 0 {
		t.Errorf("fast path should not remove orphans, removed=%d", removed)
	}
	man, _ := loadManifest(dir2)
	if _, ok := man["sub/b.md"]; !ok {
		t.Errorf("fast path should retain prior manifest entry for sub/b.md: %v", man)
	}
	_ = written
}

// ---- writeFile -------------------------------------------------------------

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	if err := writeFile(dir, "nested/deep/x.md", "hello"); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "nested", "deep", "x.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(b) != "hello\n" {
		t.Errorf("content = %q, want trailing newline added", b)
	}
	// Content already ending in newline is left as-is (no double newline).
	if err := writeFile(dir, "y.md", "hi\n"); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(dir, "y.md"))
	if string(b) != "hi\n" {
		t.Errorf("content = %q, want single newline", b)
	}
}

// ---- Sync short-circuits (no Dolt touched) ---------------------------------

func TestSyncShortCircuits(t *testing.T) {
	ctx := context.Background()
	// No targets: returns an empty stat without touching the (nil) store.
	st, err := Sync(ctx, nil, nil, DirtySet{Full: true})
	if err != nil {
		t.Fatalf("Sync no targets: %v", err)
	}
	if !st.Full || st.Written != 0 || st.Removed != 0 || len(st.Formats) != 0 {
		t.Errorf("unexpected stats: %+v", st)
	}
	// Empty dirty set with targets: also short-circuits.
	st, err = Sync(ctx, nil, []Target{{Format: "md", OutDir: t.TempDir()}}, DirtySet{})
	if err != nil {
		t.Fatalf("Sync empty dirty: %v", err)
	}
	if st.Full || st.Written != 0 {
		t.Errorf("empty dirty should do nothing: %+v", st)
	}
}
