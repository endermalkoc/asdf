package tutor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/endermalkoc/cusp/internal/importer"
)

func TestSingular(t *testing.T) {
	cases := map[string]string{
		"families":       "family",  // -ies -> -y
		"class":          "class",   // -ss unchanged
		"students":       "student", // -s dropped
		"studio-options": "studio-option",
		"attachable":     "attachable", // default: unchanged
	}
	for in, want := range cases {
		if got := singular(in); got != want {
			t.Errorf("singular(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKebab(t *testing.T) {
	if got := kebab("studioOptions"); got != "studio-options" {
		t.Errorf("kebab(studioOptions) = %q", got)
	}
	if got := kebab("Student"); got != "student" {
		t.Errorf("kebab(Student) = %q", got)
	}
}

func TestContains(t *testing.T) {
	if !contains([]string{"a", "b"}, "b") {
		t.Error("contains should find b")
	}
	if contains([]string{"a", "b"}, "z") {
		t.Error("contains should not find z")
	}
	if contains(nil, "x") {
		t.Error("contains(nil) should be false")
	}
}

func TestEntityNameFromHeading(t *testing.T) {
	cases := []struct{ h1, file, want string }{
		{"# Invoice Entity", "invoice.md", "Invoice"},
		{"# Student", "student.md", "Student"},
		{"", "transaction-recurring-schedule.md", "Transaction Recurring Schedule"},
		{"   ", "single.md", "Single"},
	}
	for _, c := range cases {
		if got := entityNameFromHeading(c.h1, c.file); got != c.want {
			t.Errorf("entityNameFromHeading(%q,%q) = %q, want %q", c.h1, c.file, got, c.want)
		}
	}
}

func TestLinkHref(t *testing.T) {
	if got := linkHref("[Student](./student.md)"); got != "./student.md" {
		t.Errorf("linkHref = %q", got)
	}
	if got := linkHref("no link here"); got != "" {
		t.Errorf("linkHref(no link) = %q, want empty", got)
	}
}

func TestFirstLinkText(t *testing.T) {
	if got := firstLinkText("[Add Student](x.md)"); got != "Add Student" {
		t.Errorf("firstLinkText = %q", got)
	}
	if got := firstLinkText("plain text"); got != "plain text" {
		t.Errorf("firstLinkText(plain) = %q", got)
	}
}

func TestItoa(t *testing.T) {
	cases := map[int]string{0: "0", 5: "5", 42: "42", -7: "-7"}
	for in, want := range cases {
		if got := itoa(in); got != want {
			t.Errorf("itoa(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestAtoiSafe(t *testing.T) {
	cases := map[string]int{"12": 12, "3abc": 3, "": 0, "x9": 0}
	for in, want := range cases {
		if got := atoiSafe(in); got != want {
			t.Errorf("atoiSafe(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestPriorityNum(t *testing.T) {
	if priorityNum("P1") != 1 || priorityNum("P4") != 4 {
		t.Error("priorityNum mismatch")
	}
}

func TestTitleCase(t *testing.T) {
	if titleCase("") != "" {
		t.Error("titleCase(\"\") should be empty")
	}
	if titleCase("enrollment") != "Enrollment" {
		t.Errorf("titleCase(enrollment) = %q", titleCase("enrollment"))
	}
}

func TestTopDir(t *testing.T) {
	if topDir("enrollment/add.md") != "enrollment" {
		t.Error("topDir with slash")
	}
	if topDir("add.md") != "" {
		t.Error("topDir without slash should be empty")
	}
}

func TestKnownMilestone(t *testing.T) {
	for _, m := range []string{"M0", "m4", "Future", "M7"} {
		if !knownMilestone(m) {
			t.Errorf("knownMilestone(%q) should be true", m)
		}
	}
	for _, m := range []string{"backlog", "tut-1", "M9"} {
		if knownMilestone(m) {
			t.Errorf("knownMilestone(%q) should be false", m)
		}
	}
}

func TestMapSpecStatus(t *testing.T) {
	cases := map[string]string{
		"":                "draft",
		"Draft":           "draft",
		"Active":          "active",
		"Reviewed":        "reviewed",
		"Retired (2024)":  "obsolete",
		"obsolete":        "obsolete",
		"Superseded by X": "obsolete",
		"something-weird": "draft",
	}
	rep := &importer.Report{}
	for in, want := range cases {
		if got := mapSpecStatus(in, rep, "ref"); got != want {
			t.Errorf("mapSpecStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitFRKey(t *testing.T) {
	prefix, num, suffix, ok := splitFRKey("ADDS-FR-012a")
	if !ok || prefix != "ADDS" || num != 12 || suffix != "a" {
		t.Errorf("splitFRKey(ADDS-FR-012a) = %q/%d/%q/%v", prefix, num, suffix, ok)
	}
	if _, _, _, ok := splitFRKey("not-a-key"); ok {
		t.Error("splitFRKey(not-a-key) should fail")
	}
	// Trailing junk means the whole string is not exactly an FR key.
	if _, _, _, ok := splitFRKey("ADDS-FR-012a extra"); ok {
		t.Error("splitFRKey with trailing text should fail")
	}
}

func TestFRKey(t *testing.T) {
	if got := frKey("ADDS", 7, ""); got != "ADDS-FR-007" {
		t.Errorf("frKey = %q", got)
	}
	if got := frKey("ATT", 30, "b"); got != "ATT-FR-030b" {
		t.Errorf("frKey suffix = %q", got)
	}
}

func TestTableCells(t *testing.T) {
	cells, ok := tableCells("| a | b | c |")
	if !ok || len(cells) != 3 || cells[0] != "a" || cells[2] != "c" {
		t.Errorf("tableCells row = %v ok=%v", cells, ok)
	}
	if _, ok := tableCells("|---|---|"); ok {
		t.Error("separator row should be rejected")
	}
	if _, ok := tableCells("not a table"); ok {
		t.Error("non-table line should be rejected")
	}
}

func TestNormHeading(t *testing.T) {
	if got := normHeading("Requirements _(mandatory)_"); got != "requirements" {
		t.Errorf("normHeading mandatory = %q", got)
	}
	if got := normHeading("Overview (Optional)"); got != "overview" {
		t.Errorf("normHeading optional = %q", got)
	}
}

func TestTrimMarkdown(t *testing.T) {
	if got := trimMarkdown("  hello  "); got != "hello" {
		t.Errorf("trimMarkdown = %q", got)
	}
}

func TestSplitAnchor(t *testing.T) {
	base, anchor := splitAnchor("file.md#section")
	if base != "file.md" || anchor != "section" {
		t.Errorf("splitAnchor = %q/%q", base, anchor)
	}
	base, anchor = splitAnchor("file.md")
	if base != "file.md" || anchor != "" {
		t.Errorf("splitAnchor no anchor = %q/%q", base, anchor)
	}
}

func TestIsRelMD(t *testing.T) {
	c := newRefConverter(&importer.Graph{})
	rel := map[string]bool{
		"./other.md":     true,
		"../a/b.md":      true,
		"x.md#anchor":    true,
		"http://x/y.md":  false,
		"https://x/y.md": false,
		"mailto:a@b.com": false,
		"#anchor":        false,
		"/abs/path.md":   false,
		"notes.txt":      false,
	}
	for url, want := range rel {
		if got := c.isRelMD(url); got != want {
			t.Errorf("isRelMD(%q) = %v, want %v", url, got, want)
		}
	}
}

func TestStripFrontmatter(t *testing.T) {
	got := stripFrontmatter("---\nid: X\n---\n# Title\nbody")
	if got != "# Title\nbody" {
		t.Errorf("stripFrontmatter = %q", got)
	}
	// No frontmatter: unchanged.
	if got := stripFrontmatter("# Title\nbody"); got != "# Title\nbody" {
		t.Errorf("stripFrontmatter(no fm) = %q", got)
	}
	// Unterminated frontmatter: returned as-is.
	in := "---\nid: X\nno close"
	if got := stripFrontmatter(in); got != in {
		t.Errorf("stripFrontmatter(unterminated) = %q", got)
	}
}

func TestFrontmatterBlockAndReadFrontmatter(t *testing.T) {
	block, ok := frontmatterBlock("\n---\nid: A\ntitle: T\n---\nrest")
	if !ok || block != "id: A\ntitle: T" {
		t.Errorf("frontmatterBlock = %q ok=%v", block, ok)
	}
	if _, ok := frontmatterBlock("no frontmatter"); ok {
		t.Error("frontmatterBlock(no fm) should be false")
	}
	if _, ok := frontmatterBlock("---\nunterminated"); ok {
		t.Error("frontmatterBlock(unterminated) should be false")
	}

	// readFrontmatter on a real file.
	dir := t.TempDir()
	p := filepath.Join(dir, "s.md")
	if err := os.WriteFile(p, []byte("---\nid: ADDS\ntitle: Add\n---\n# Add"), 0o644); err != nil {
		t.Fatal(err)
	}
	fm, ok := readFrontmatter(p)
	if !ok || fm.ID != "ADDS" || fm.Title != "Add" {
		t.Errorf("readFrontmatter = %+v ok=%v", fm, ok)
	}
	if _, ok := readFrontmatter(filepath.Join(dir, "missing.md")); ok {
		t.Error("readFrontmatter(missing) should be false")
	}
	// A file with no frontmatter block.
	np := filepath.Join(dir, "plain.md")
	if err := os.WriteFile(np, []byte("# Just a heading"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := readFrontmatter(np); ok {
		t.Error("readFrontmatter(no fm) should be false")
	}
}

func TestSplitDocAndSubsection(t *testing.T) {
	body := "# Title\npreamble line\n\n## Overview\nintro\n### Sub\nsub body\n## Second\nsecond body\n"
	h1, preamble, secs := splitDoc(body)
	if h1 != "Title" {
		t.Errorf("h1 = %q", h1)
	}
	if preamble != "preamble line" {
		t.Errorf("preamble = %q", preamble)
	}
	if len(secs) != 2 || secs[0].heading != "Overview" || secs[1].heading != "Second" {
		t.Fatalf("sections = %+v", secs)
	}
	if got := subsection(secs[0].body, "sub"); got != "sub body" {
		t.Errorf("subsection(sub) = %q", got)
	}
	if got := subsection(secs[0].body, "missing"); got != "" {
		t.Errorf("subsection(missing) = %q, want empty", got)
	}

	// A doc that opens straight with a `##` (no H1).
	h1, _, secs2 := splitDoc("## Only Section\nbody\n")
	if h1 != "" || len(secs2) != 1 {
		t.Errorf("splitDoc(no h1) h1=%q secs=%d", h1, len(secs2))
	}
}

func TestParseKeyEntities(t *testing.T) {
	body := "- **Student**: a learner\n- **Family**: a household\n- **Student**: duplicate\nplain line\n"
	got := parseKeyEntities(body)
	if len(got) != 2 || got[0] != "Student" || got[1] != "Family" {
		t.Errorf("parseKeyEntities = %v", got)
	}
}

func TestCleanSpecPreamble(t *testing.T) {
	pre := "**Feature Branch**: feat/x\n**Status**: Active\n**Created**: 2024-01-02\n**Updated**: 2024-02-02\n**Input**: keep me\nfree prose"
	cleaned, created := cleanSpecPreamble(pre)
	if created != "2024-01-02" {
		t.Errorf("created = %q", created)
	}
	if contains2(cleaned, "Feature Branch") || contains2(cleaned, "Status") || contains2(cleaned, "Updated") {
		t.Errorf("cleaned kept boilerplate: %q", cleaned)
	}
	if !contains2(cleaned, "Input") || !contains2(cleaned, "free prose") {
		t.Errorf("cleaned dropped real content: %q", cleaned)
	}
}

func TestResolveDrizzleDir(t *testing.T) {
	root := t.TempDir()

	// Explicit override that is an existing directory -> returned.
	schema := filepath.Join(root, "schema")
	if err := os.MkdirAll(schema, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := resolveDrizzleDir(root, schema); got != schema {
		t.Errorf("resolveDrizzleDir(existing dir) = %q, want %q", got, schema)
	}

	// Override that is a file (not a dir) -> "".
	f := filepath.Join(root, "afile")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := resolveDrizzleDir(root, f); got != "" {
		t.Errorf("resolveDrizzleDir(file override) = %q, want empty", got)
	}

	// Override that does not exist -> "".
	if got := resolveDrizzleDir(root, filepath.Join(root, "nope")); got != "" {
		t.Errorf("resolveDrizzleDir(missing override) = %q, want empty", got)
	}

	// No override, auto-location absent -> "".
	if got := resolveDrizzleDir(root, ""); got != "" {
		t.Errorf("resolveDrizzleDir(auto absent) = %q, want empty", got)
	}
}

func TestResolveDrizzleDir_AutoDetect(t *testing.T) {
	// Build the conventional auto path relative to a docs root and confirm it is found.
	base := t.TempDir()
	docsRoot := filepath.Join(base, "docs")
	auto := filepath.Join(docsRoot, "..", "src", "packages", "database", "src", "schema")
	if err := os.MkdirAll(auto, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(docsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	got := resolveDrizzleDir(docsRoot, "")
	if got != auto {
		t.Errorf("resolveDrizzleDir(auto) = %q, want %q", got, auto)
	}
}

func TestCrossCheckCoverage(t *testing.T) {
	g := &importer.Graph{Reqs: make([]importer.Requirement, 3)}

	// No summary file -> no findings.
	dir := t.TempDir()
	rep := &importer.Report{}
	crossCheckCoverage(dir, g, rep)
	if len(rep.Findings) != 0 {
		t.Errorf("no summary should yield no findings, got %+v", rep.Findings)
	}

	// Matching total -> no drift finding.
	write := func(content string) string {
		d := t.TempDir()
		if err := os.WriteFile(filepath.Join(d, "fr-coverage-summary.json"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return d
	}
	rep = &importer.Report{}
	crossCheckCoverage(write(`{"total": 3}`), g, rep)
	if len(rep.Findings) != 0 {
		t.Errorf("matching total should yield no findings, got %+v", rep.Findings)
	}

	// Mismatched total -> one drift finding.
	rep = &importer.Report{}
	crossCheckCoverage(write(`{"total": 99}`), g, rep)
	if !findingCategories(rep)["coverage-count-drift"] {
		t.Error("mismatched total should emit coverage-count-drift")
	}

	// Invalid JSON -> no finding.
	rep = &importer.Report{}
	crossCheckCoverage(write(`not json`), g, rep)
	if len(rep.Findings) != 0 {
		t.Errorf("invalid JSON should yield no findings, got %+v", rep.Findings)
	}

	// total == 0 -> ignored.
	rep = &importer.Report{}
	crossCheckCoverage(write(`{"total": 0}`), g, rep)
	if len(rep.Findings) != 0 {
		t.Errorf("total 0 should yield no findings, got %+v", rep.Findings)
	}
}

func TestSectionExtras(t *testing.T) {
	body := "intro before headings\n### Keep This\nkept body\n### Skip Me\nskipped body\n"
	extras := sectionExtras(body, func(h string) bool { return h == "skip me" })
	// intro (no heading) + "Keep This"; "Skip Me" is skipped.
	if len(extras) != 2 {
		t.Fatalf("sectionExtras = %d entries: %+v", len(extras), extras)
	}
	if extras[0].heading != "" || extras[0].body != "intro before headings" {
		t.Errorf("intro extra = %+v", extras[0])
	}
	if extras[1].heading != "Keep This" || extras[1].body != "kept body" {
		t.Errorf("kept extra = %+v", extras[1])
	}
}

func TestSectionAccFold(t *testing.T) {
	a := newSectionAcc()
	a.fold("A Heading", "body text")
	a.fold("", "headless body")
	a.fold("Empty", "") // no-op (empty body under a heading)
	secs := a.sections()
	if len(secs) != 1 || secs[0].Key != "notes" {
		t.Fatalf("sections = %+v", secs)
	}
	if !contains2(secs[0].Body, "### A Heading") || !contains2(secs[0].Body, "headless body") {
		t.Errorf("folded notes body = %q", secs[0].Body)
	}
}

func TestDedupeSpecsByPath(t *testing.T) {
	in := []importer.Spec{
		{Prefix: "A", Path: "x.md"},
		{Prefix: "B", Path: "x.md"}, // dup path -> dropped
		{Prefix: "C", Path: "y.md"},
	}
	out := dedupeSpecsByPath(in)
	if len(out) != 2 || out[0].Prefix != "A" || out[1].Prefix != "C" {
		t.Errorf("dedupeSpecsByPath = %+v", out)
	}
}

func TestParseDrizzleTables_SingleTable(t *testing.T) {
	content := `export const foo = pgTable("foo_snake", {
  id: uuid("id").primaryKey(),
  barId: uuid("bar_id").references(() => bar.id),
}, (table) => ({
  pk: primaryKey({ columns: [table.id, table.barId] }),
}));`
	tables := parseDrizzleTables(content)
	if len(tables) != 1 {
		t.Fatalf("tables = %d", len(tables))
	}
	tb := tables[0]
	if tb.camel != "foo" || tb.snake != "foo_snake" {
		t.Errorf("names = %q/%q", tb.camel, tb.snake)
	}
	if len(tb.fks) != 1 || tb.fks[0].col != "barId" || tb.fks[0].target != "bar" {
		t.Errorf("fks = %+v", tb.fks)
	}
	if len(tb.pkCols) != 2 {
		t.Errorf("pkCols = %+v", tb.pkCols)
	}
}
