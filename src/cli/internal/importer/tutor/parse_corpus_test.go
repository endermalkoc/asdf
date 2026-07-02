package tutor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/endermalkoc/cusp/internal/importer"
)

// corpus is a small but representative tutor corpus written to a temp dir so Parse
// runs end-to-end without depending on the external ../tutor tree. Every file is
// crafted to exercise a distinct code path (drift findings, FR groups, ranges,
// tombstones, stories, scenarios, entities, cross-references).
var corpus = map[string]string{
	"specs/index.md": `
# Index of Domains

| Domain | Description |
|--------|-------------|
| enrollment/ | Student enrollment flows |
| attendance/ | Attendance tracking |
| entities/ | Entity glossary (not a real domain) |
| bad slug/ | has a space, skipped |
| enrollment/ | duplicate, skipped |

Some prose between the two tables.

| Directory | Notes |
|-----------|-------|
| other/ | Miscellaneous specs |
`,

	"specs/prefix-registry.md": `
# Prefix Registry

| Prefix | Path | Domain | Status |
|--------|------|--------|--------|
| ADDS | enrollment/add-student.md | enrollment | Active |
| ATT | attendance/track.md | attendance | Reviewed |
| ATCH | entities/attachable.md | attendance | Draft |
| GONE | enrollment/missing.md | enrollment | Retired |
| BAD | other/bad.md | nosuchdomain | Draft |
`,

	"specs/enrollment/add-student.md": `
---
id: ADDS
title: Add Student
domain: enrollment
status: Active
---

# Add Student Feature

**Feature Branch**: feat/add-student
**Status**: Active
**Created**: 2024-01-15
**Updated**: 2024-02-01
**Input**: User description: add a student to the roster

Intro prose paragraph.

## Overview

Enroll students into the roster.

See [Track Attendance](../attendance/track.md), the [Student entity](../entities/student.md#purpose), requirement [ADDS-FR-001](./add-student.md#adds-fr-001), a [Broken link](./does-not-exist.md), and [Google](https://example.com).

## Success Criteria

- Students can be added in under a minute.

## Platform Scope

Web and mobile.

## Assumptions

Roster exists.

## Clarifications

- Q: What about middle names?

## User Scenarios & Testing

### User Story 1 - Add a student (Priority: P1)

As a tutor, I want to add a student
so that I can track their progress.

**Why this priority**: Core onboarding flow that
wraps across lines.
**Independent Test**: Can be tested by creating a student.

**Acceptance Scenarios**

1. **Given** the roster is open **When** I click add **Then** a form appears
2. **Given** a valid form, **Then** the student is saved

### User Story 2 - Prose only story (Priority: P2)

This story has no narrative, just prose describing the behavior.

### User Story 3 - Tricky narrative (Priority: P3)

I want something but there is no proper prefix here.

### Edge Cases

- What happens when the name is empty?

### Extra Subsection

Some extra prose that folds to notes.

## Requirements

### Key Entities

- **Student**: a learner
- **Family**: a household

### Functional Requirements

**Student Section**

Group note prose.

- **ADDS-FR-001**: System MUST require first name and last name
  continuation line
- **ADDS-FR-002**: [visual] Red dot on unsaved rows
- **ADDS-FR-002**: duplicate line to trigger duplicate finding
- **ADDS-FR-003 to ADDS-FR-005**: Email and phone validation follow [shared/contacts.md]
- **ADDS-FR-004 to XYZ-FR-006**: mismatched range that fails

#### Family Section (M4)

- **ADDS-FR-006**: System MUST link a family per ADDS-FR-001
- **ADDS-FR-010a**: Suffix variant requirement
- **ADDS-FR-099**: _(tombstoned)_ intentionally omitted
- **ADDS-FR-050**: An orphan requirement with no registry entry

**Note Only Config**

Just config prose, no FR follows.

## Notes

Freeform trailing notes.
`,

	"specs/attendance/track.md": `
---
id: ATTENDANCE
title: Track Attendance
domain: attendance
status: Reviewed
---

# Track Attendance

**Created**: 2024-03-01

## Requirements

### Functional Requirements

- **ATT-FR-001**: System MUST record attendance
`,

	"specs/other/bad.md": `
---
id: BAD
title: Bad Spec
domain: nosuchdomain
status: Draft
---

# Bad Spec

## Overview

Nothing special here.
`,

	"specs/enrollment/extra.md": `
---
id: EXTRA
title: An Unregistered Spec
---

# Extra
`,

	"specs/entities/index.md": `
# Entity Glossary

| Entity | Description | Status |
|--------|-------------|--------|
| [Student](./student.md) | A learner in the system | Active |
| [Family](./family.md) | A household grouping | Draft |
`,

	"specs/entities/student.md": `
# Student Entity

Student preamble prose.

## Purpose

Represents a learner.

## Key Concepts

- Enrollment status

## Schema Reference

See the students table.

## Relationships

Belongs to a [Family](./family.md).

## Business Rules

Must have a name.

## Validations

Name is required.

## Row-Level Access Rules

Studio-scoped access.

## Notes

Some notes.

## Spec References

See add-student.

## Custom Heading

Folds into notes.
`,

	"specs/entities/family.md": `
# Family Entity

A household grouping.

## Purpose

Groups students together.
`,

	"specs/entities/attachable.md": `
# Attachable Entity

Attachable preamble.

## Purpose

Files that attach to records.
`,

	"fr-registry/adds.yml": `
ADDS-FR-001:
  status: covered
  milestone: M4
  notes: core requirement
ADDS-FR-002:
  status: test-pending
  milestone: backlog
ADDS-FR-003:
  status: shared
ADDS-FR-004:
  status: not-implemented
  milestone: tut-123
ADDS-FR-005:
  status: deferred
ADDS-FR-006:
  status: covered
  e2e_ref: e2e/add-student.spec.ts
ADDS-FR-010a:
  status: schema-only
ADDS-FR-099:
  status: e2e-sufficient
ADDS-FR-200:
  status: made-up-status
  milestone: M5
not-a-valid-key:
  status: covered
`,

	"fr-registry/att.yml": `
ATT-FR-001:
  status: covered
  milestone: M1
`,

	"fr-registry/bad.yml": `
- this is a yaml sequence, not a map
`,

	"fr-coverage-summary.json": `{"total": 1}`,
}

// drizzleSchema is a Drizzle (TypeScript) schema exercising the FK/junction cases:
// one-to-many (student -> family FK), one-to-one (attachable id FK), many-to-many
// (student_family junction), and an N-ary junction that is skipped.
const drizzleSchema = `
export const student = pgTable("student", {
  id: uuid("id").primaryKey(),
  familyId: uuid("family_id").references(() => family.id),
});

export const family = pgTable("family", {
  id: uuid("id").primaryKey(),
});

export const attachable = pgTable("attachable", {
  id: uuid("id").references(() => student.id),
});

export const studentFamily = pgTable("student_family", {
  studentId: uuid("student_id").references(() => student.id),
  familyId: uuid("family_id").references(() => family.id),
}, (table) => ({
  pk: primaryKey({ columns: [table.studentId, table.familyId] }),
}));

export const ternaryJunction = pgTable("ternary_junction", {
  studentId: uuid("student_id").references(() => student.id),
  familyId: uuid("family_id").references(() => family.id),
  attachableId: uuid("attachable_id").references(() => attachable.id),
}, (table) => ({
  pk: primaryKey({ columns: [table.studentId, table.familyId, table.attachableId] }),
}));
`

// writeCorpus materializes the fixture corpus + Drizzle schema under t.TempDir()
// and returns (docsRoot, drizzleDir).
func writeCorpus(t *testing.T) (docsRoot, drizzleDir string) {
	t.Helper()
	docsRoot = t.TempDir()
	for rel, content := range corpus {
		p := filepath.Join(docsRoot, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	drizzleDir = filepath.Join(docsRoot, "schema")
	if err := os.MkdirAll(drizzleDir, 0o755); err != nil {
		t.Fatalf("mkdir schema: %v", err)
	}
	if err := os.WriteFile(filepath.Join(drizzleDir, "schema.ts"), []byte(drizzleSchema), 0o644); err != nil {
		t.Fatalf("write schema.ts: %v", err)
	}
	return docsRoot, drizzleDir
}

func findingCategories(rep *importer.Report) map[string]bool {
	set := map[string]bool{}
	for _, f := range rep.Findings {
		set[f.Category] = true
	}
	return set
}

func TestParse_Corpus_CountsAndEntities(t *testing.T) {
	docsRoot, drizzleDir := writeCorpus(t)

	g, rep, err := Parse(docsRoot, drizzleDir)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if g == nil || rep == nil {
		t.Fatal("Parse returned nil graph or report")
	}

	// Domains: enrollment, attendance, other (entities/duplicate/bad-slug dropped).
	if got := len(g.Domains); got != 3 {
		t.Errorf("domains = %d, want 3 (%+v)", got, g.Domains)
	}
	if rep.Counts["domains"] != 3 {
		t.Errorf("Counts[domains] = %d, want 3", rep.Counts["domains"])
	}

	// Specs: ADDS, ATT, GONE, BAD (ATCH under entities/ is not a spec).
	if got := len(g.Specs); got != 4 {
		t.Errorf("specs = %d, want 4", got)
	}
	for _, sp := range g.Specs {
		if sp.Prefix == "ATCH" {
			t.Error("entities/ registry row ATCH must not become a spec")
		}
	}

	// Requirements: 9 from adds.yml (valid keys) + 1 from att.yml = 10.
	if got := len(g.Reqs); got != 10 {
		t.Errorf("requirements = %d, want 10", got)
	}

	// Entities: Student, Family (index) + Attachable (unlisted on disk).
	if got := len(g.Entities); got != 3 {
		t.Errorf("entities = %d, want 3 (%+v)", got, entityNames(g))
	}
	if !hasEntity(g, "Student") || !hasEntity(g, "Family") || !hasEntity(g, "Attachable") {
		t.Errorf("missing expected entity, have %v", entityNames(g))
	}

	// Milestones: M1, M4, M5 (registry), backlog, tut-123. tut-* stays a milestone
	// value in the staging graph (reclassified to an external ref only at apply).
	if got := len(g.Milestones); got != 5 {
		t.Errorf("milestones = %d, want 5 (%+v)", got, g.Milestones)
	}

	// Stories & scenarios from add-student.md.
	if got := len(g.Stories); got != 3 {
		t.Errorf("user stories = %d, want 3", got)
	}
	if got := len(g.Scenarios); got != 2 {
		t.Errorf("scenarios = %d, want 2", got)
	}

	// Entity relationships from the Drizzle schema: one_to_many, one_to_one, many_to_many.
	if got := len(g.Relationships); got != 3 {
		t.Errorf("relationships = %d, want 3 (%+v)", got, g.Relationships)
	}
}

func TestParse_Corpus_RequirementEnrichment(t *testing.T) {
	docsRoot, drizzleDir := writeCorpus(t)
	g, _, err := Parse(docsRoot, drizzleDir)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	r001 := findReq(g, "ADDS-FR-001")
	if r001 == nil {
		t.Fatal("ADDS-FR-001 not found")
	}
	if r001.SpecPrefix != "ADDS" || r001.Number != 1 {
		t.Errorf("ADDS-FR-001 prefix/number = %q/%d", r001.SpecPrefix, r001.Number)
	}
	if !contains2(r001.Statement, "first name") {
		t.Errorf("ADDS-FR-001 statement missing 'first name': %q", r001.Statement)
	}
	if !contains2(r001.Statement, "continuation line") {
		t.Errorf("ADDS-FR-001 statement missing continuation: %q", r001.Statement)
	}
	if r001.DeliveryStatus != "covered" || r001.Milestone != "M4" {
		t.Errorf("ADDS-FR-001 status/milestone = %q/%q", r001.DeliveryStatus, r001.Milestone)
	}
	if r001.Section != "Student Section" {
		t.Errorf("ADDS-FR-001 group section = %q, want Student Section", r001.Section)
	}

	// Suffix requirement.
	r010a := findReq(g, "ADDS-FR-010a")
	if r010a == nil || r010a.Suffix != "a" || r010a.Number != 10 {
		t.Errorf("ADDS-FR-010a not parsed correctly: %+v", r010a)
	}

	// Tombstoned requirement.
	r099 := findReq(g, "ADDS-FR-099")
	if r099 == nil || !r099.Tombstoned {
		t.Errorf("ADDS-FR-099 should be tombstoned: %+v", r099)
	}

	// Delegated range requirement has the shared statement.
	r004 := findReq(g, "ADDS-FR-004")
	if r004 == nil || !contains2(r004.Statement, "Email and phone") {
		t.Errorf("ADDS-FR-004 (range) statement = %+v", r004)
	}

	// Registry-only requirement with no spec line has an empty statement.
	r200 := findReq(g, "ADDS-FR-200")
	if r200 == nil || r200.Statement != "" {
		t.Errorf("ADDS-FR-200 should have empty statement: %+v", r200)
	}

	// FR groups on the spec.
	adds := findSpec(g, "ADDS")
	if adds == nil {
		t.Fatal("ADDS spec not found")
	}
	if adds.Created != "2024-01-15" {
		t.Errorf("ADDS created = %q, want 2024-01-15", adds.Created)
	}
	if len(adds.ReqGroups) != 2 {
		t.Fatalf("ADDS req groups = %d, want 2 (%+v)", len(adds.ReqGroups), adds.ReqGroups)
	}
	if adds.ReqGroups[0].Title != "Student Section" || adds.ReqGroups[0].Notes != "Group note prose." {
		t.Errorf("group 0 = %+v", adds.ReqGroups[0])
	}
	if adds.ReqGroups[1].Title != "Family Section (M4)" {
		t.Errorf("group 1 title = %q", adds.ReqGroups[1].Title)
	}
	if !hasSectionKey(adds.Sections, "more_info") {
		t.Errorf("ADDS should carry a more_info section, have %v", sectionKeys(adds.Sections))
	}
	if !hasSectionKey(adds.Sections, "overview") || !hasSectionKey(adds.Sections, "scope") {
		t.Errorf("ADDS missing curated sections, have %v", sectionKeys(adds.Sections))
	}
	if len(adds.KeyEntities) != 2 {
		t.Errorf("ADDS key entities = %v, want [Student Family]", adds.KeyEntities)
	}
}

func TestParse_Corpus_StoriesAndScenarios(t *testing.T) {
	docsRoot, drizzleDir := writeCorpus(t)
	g, _, err := Parse(docsRoot, drizzleDir)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	s1 := findStory(g, 1)
	if s1 == nil {
		t.Fatal("story 1 not found")
	}
	if s1.Title != "Add a student" || s1.Priority != 1 {
		t.Errorf("story 1 title/priority = %q/%d", s1.Title, s1.Priority)
	}
	if s1.AsA != "tutor" {
		t.Errorf("story 1 as_a = %q, want tutor", s1.AsA)
	}
	if !contains2(s1.IWant, "add a student") {
		t.Errorf("story 1 i_want = %q", s1.IWant)
	}
	if !contains2(s1.SoThat, "track their progress") {
		t.Errorf("story 1 so_that = %q", s1.SoThat)
	}
	if !contains2(s1.WhyPriority, "wraps across lines") {
		t.Errorf("story 1 why_priority did not gather wrapped lines: %q", s1.WhyPriority)
	}
	if !contains2(s1.IndependentTest, "creating a student") {
		t.Errorf("story 1 independent_test = %q", s1.IndependentTest)
	}

	// Prose-style story (no narrative parsed) keeps its lead prose.
	s2 := findStory(g, 2)
	if s2 == nil || s2.IWant != "" || s2.Narrative == "" {
		t.Errorf("story 2 should be prose-style with narrative: %+v", s2)
	}

	// Scenarios: one with a When clause, one without.
	var withWhen, withoutWhen *importer.Scenario
	for i := range g.Scenarios {
		sc := &g.Scenarios[i]
		if sc.When != "" {
			withWhen = sc
		} else {
			withoutWhen = sc
		}
	}
	if withWhen == nil || !contains2(withWhen.When, "click add") {
		t.Errorf("scenario with When missing: %+v", withWhen)
	}
	if withoutWhen == nil || withoutWhen.Given != "a valid form" {
		t.Errorf("Given/Then-only scenario mis-parsed (want trailing comma trimmed): %+v", withoutWhen)
	}
}

func TestParse_Corpus_RefsAndTokens(t *testing.T) {
	docsRoot, drizzleDir := writeCorpus(t)
	g, _, err := Parse(docsRoot, drizzleDir)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// A requirement's inline "per ADDS-FR-001" mention becomes a requirement->requirement ref.
	if !hasRef(g, "requirement", "ADDS-FR-006", "requirement", "ADDS-FR-001") {
		t.Error("missing requirement->requirement ref ADDS-FR-006 -> ADDS-FR-001")
	}
	// Key Entities produce spec->entity refs.
	if !hasRef(g, "spec", "enrollment/add-student.md", "entity", "Student") {
		t.Error("missing spec->entity ref for Student")
	}
	// The Overview cross-spec markdown link resolves to the ATT spec.
	if !hasRef(g, "spec", "enrollment/add-student.md", "spec", "attendance/track.md") {
		t.Error("missing spec->spec ref for the converted markdown link")
	}
	// The entity Relationships prose link resolves entity->entity.
	if !hasRef(g, "entity", "Student", "entity", "Family") {
		t.Error("missing entity->entity ref Student -> Family")
	}

	// The requirement statement was canonicalized: the bare id is now a token.
	r006 := findReq(g, "ADDS-FR-006")
	if r006 == nil || !contains2(r006.Statement, "[[REQ:ADDS-FR-001]]") {
		t.Errorf("ADDS-FR-006 statement not canonicalized: %q", r006.Statement)
	}
}

func TestParse_Corpus_Findings(t *testing.T) {
	docsRoot, drizzleDir := writeCorpus(t)
	_, rep, err := Parse(docsRoot, drizzleDir)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cats := findingCategories(rep)
	want := []string{
		"domain-description",
		"prefix-mismatch",
		"registry-missing-file",
		"unknown-domain",
		"cross-domain-spec",
		"unregistered-spec",
		"duplicate-fr-line",
		"bad-fr-range",
		"orphan-fr-line",
		"tombstoned-fr",
		"spec-status-reviewed",
		"entity-doc-unlisted",
		"entity-attributes-deferred",
		"bad-fr-key",
		"registry-no-statement",
		"registry-e2e-ref",
		"unknown-delivery-status",
		"bad-registry-yaml",
		"extra-milestone",
		"milestone-is-issue-id",
		"coverage-count-drift",
		"drizzle-relationships",
		"drizzle-ternary-junction",
		"unresolved-ref",
		"story-narrative-unparsed",
		"story-prose-style",
	}
	for _, c := range want {
		if !cats[c] {
			t.Errorf("expected finding category %q not present", c)
		}
	}

	// Coverage histogram was populated (delivery statuses tallied).
	if rep.Coverage["covered"] == 0 {
		t.Errorf("coverage histogram missing 'covered': %+v", rep.Coverage)
	}
}

// TestParse_DrizzleOverrideMissing hits the branch where an explicit --drizzle dir
// does not exist: no relationships, plus a drizzle-not-found warning.
func TestParse_DrizzleOverrideMissing(t *testing.T) {
	docsRoot, _ := writeCorpus(t)
	g, rep, err := Parse(docsRoot, filepath.Join(docsRoot, "no-such-schema-dir"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(g.Relationships) != 0 {
		t.Errorf("relationships = %d, want 0 with a missing drizzle dir", len(g.Relationships))
	}
	if !findingCategories(rep)["drizzle-not-found"] {
		t.Error("expected drizzle-not-found finding for a missing override dir")
	}
}

// TestParse_NoSpecsDir returns an error when specs/ is absent.
func TestParse_NoSpecsDir(t *testing.T) {
	root := t.TempDir()
	if _, _, err := Parse(root, ""); err == nil {
		t.Fatal("expected an error when specs/ is missing")
	}
}

// TestParse_NoPrefixRegistry surfaces the parseSpecs read error.
func TestParse_NoPrefixRegistry(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Parse(root, ""); err == nil {
		t.Fatal("expected an error when prefix-registry.md is missing")
	}
}

// ---- small assertion helpers ----------------------------------------------

func contains2(haystack, needle string) bool {
	return len(needle) == 0 || indexOf(haystack, needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func entityNames(g *importer.Graph) []string {
	var out []string
	for _, e := range g.Entities {
		out = append(out, e.Name)
	}
	return out
}

func hasEntity(g *importer.Graph, name string) bool {
	for _, e := range g.Entities {
		if e.Name == name {
			return true
		}
	}
	return false
}

func findReq(g *importer.Graph, key string) *importer.Requirement {
	for i := range g.Reqs {
		if g.Reqs[i].FRKey == key {
			return &g.Reqs[i]
		}
	}
	return nil
}

func findSpec(g *importer.Graph, prefix string) *importer.Spec {
	for i := range g.Specs {
		if g.Specs[i].Prefix == prefix {
			return &g.Specs[i]
		}
	}
	return nil
}

func findStory(g *importer.Graph, position int) *importer.UserStory {
	for i := range g.Stories {
		if g.Stories[i].Position == position {
			return &g.Stories[i]
		}
	}
	return nil
}

func hasRef(g *importer.Graph, ot, ok, tt, tk string) bool {
	for _, r := range g.Refs {
		if r.OwnerType == ot && r.OwnerKey == ok && r.TargetType == tt && r.TargetKey == tk {
			return true
		}
	}
	return false
}

func hasSectionKey(secs []importer.DocSection, key string) bool {
	for _, s := range secs {
		if s.Key == key {
			return true
		}
	}
	return false
}

func sectionKeys(secs []importer.DocSection) []string {
	var out []string
	for _, s := range secs {
		out = append(out, s.Key)
	}
	return out
}
