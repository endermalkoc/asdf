// Package importer holds the source-agnostic core of ASDF's import pipeline:
// the staging Graph (a corpus parsed into ASDF's entity shapes, keyed by
// business identifiers rather than minted DB ids) and the Report a read-only
// parse produces. Source adapters (e.g. internal/importer/tutor) populate a
// Graph; a later write pass resolves the business keys to rows through the
// store/Mutate command contract.
//
// This first cut is parse-and-report only: nothing here touches the database.
package importer

// Graph is a source corpus parsed into ASDF's entity shapes. Rows are keyed by
// business identifiers (domain abbreviation, spec prefix, fr_key, …) because at
// parse time no ULIDs or foreign keys exist yet.
type Graph struct {
	Domains    []Domain      `json:"domains"`
	Specs      []Spec        `json:"specs"`
	Reqs       []Requirement `json:"requirements"`
	Stories    []UserStory   `json:"user_stories"`
	Scenarios  []Scenario    `json:"acceptance_scenarios"`
	Refs       []EntityRef   `json:"entity_refs"`
	Milestones []Milestone   `json:"milestones"`
	Entities   []Entity      `json:"entities"`
}

// Entity ← a row of specs/entities/index.md (the entity glossary). Its DocPath
// points at the kind=entity Spec that documents it.
type Entity struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`   // mapped to entity.status (draft|active|deprecated)
	DocPath     string `json:"doc_path"` // entities/<file>.md

	// All entity-doc sections (verbatim Markdown) → doc_section: recognized ones
	// (purpose, key_concepts, …) carry a section_key; bespoke ones carry "".
	Sections []DocSection `json:"sections,omitempty"`
}

// Domain ← a top-level directory under specs/, described in specs/index.md.
type Domain struct {
	Abbrev      string `json:"abbrev"`      // directory name, e.g. "enrollment"
	Name        string `json:"name"`        // title-cased label
	Kind        string `json:"kind"`        // service|shared|entities|infrastructure
	Description string `json:"description"` // from index.md → domain.description (added in 0002)
}

// Spec ← a row of specs/prefix-registry.md, enriched from the file's frontmatter
// and its document sections (for an information-complete round-trip).
type Spec struct {
	Prefix    string `json:"prefix"`     // e.g. ADDS (empty for FR-exempt docs)
	Path      string `json:"path"`       // relative to specs/, e.g. enrollment/add-student.md
	Title     string `json:"title"`      // frontmatter title
	Domain    string `json:"domain"`     // domain abbrev (registry column)
	Kind      string `json:"kind"`       // feature|entity|journey|index|meta|reference
	RawStatus string `json:"raw_status"` // source status verbatim: Draft|Reviewed|Active
	Status    string `json:"status"`     // mapped to ASDF spec.status (draft|active|obsolete)

	// Heading is the H1 identity line (kept on spec). All prose sections →
	// doc_section: recognized ones (overview, edge_cases, success_criteria,
	// platform_scope, assumptions, clarifications, preamble, more_info) carry a
	// section_key; bespoke ones carry "".
	Heading     string       `json:"heading,omitempty"`
	KeyEntities []string     `json:"key_entities,omitempty"` // entity names → spec→entity refs
	Sections    []DocSection `json:"sections,omitempty"`
	ReqGroups   []ReqGroup   `json:"req_groups,omitempty"` // FR group sub-headers + notes
}

// ReqGroup is a bold FR sub-header that organizes a spec's FR list, with the
// interspersed prose (e.g. a "> See [shared/X]" blockquote) under it.
type ReqGroup struct {
	Position int    `json:"position"`
	Header   string `json:"header"`
	Note     string `json:"note,omitempty"`
}

// DocSection is a document section preserved verbatim. Key is the normalized
// section id for a recognized section (overview, edge_cases, purpose, …) or "" for
// a bespoke one. For keyed sections only Body is load-bearing (the generator renders
// them at a canonical position); Level/Heading matter only for bespoke sections.
type DocSection struct {
	Ordinal int    `json:"ordinal"`
	Level   int    `json:"level"`
	Heading string `json:"heading"`
	Body    string `json:"body"`
	Key     string `json:"key,omitempty"`
}

// Requirement ← an fr-registry/**.yml entry (authoritative existence + status),
// with Statement enriched from the spec's bold FR line.
type Requirement struct {
	FRKey          string `json:"fr_key"`             // ADDS-FR-001
	SpecPrefix     string `json:"spec_prefix"`        // ADDS
	Number         int    `json:"number"`             // 1
	Suffix         string `json:"suffix"`             // optional sub-letter
	Statement      string `json:"statement"`          // from the spec bold line ("" => drift)
	DeliveryStatus string `json:"delivery_status"`    // registry status (covered|not-implemented|…)
	Milestone      string `json:"milestone"`          // registry milestone (M1.., backlog, tut-…)
	OptoutMarker   string `json:"optout_marker"`      // none|visual|ops|untestable (from [..] tag)
	Owner          string `json:"owner,omitempty"`    // registry owner → requirement.owner
	E2ERef         string `json:"e2e_ref,omitempty"`  // registry e2e_ref → folded into notes (test linkage)
	Section        string `json:"section,omitempty"`  // FR group header it belongs to (link key)
	Position       int    `json:"position,omitempty"` // document order within the spec's FR list
	Notes          string `json:"notes"`              // registry notes
	Tombstoned     bool   `json:"tombstoned"`         // spec line marks an intentional omission
}

// UserStory ← a "### User Story N - Title (Priority: PN)" heading + its As-a line.
type UserStory struct {
	SpecPrefix      string `json:"spec_prefix"`
	Ordinal         int    `json:"ordinal"`
	Title           string `json:"title"`
	Priority        string `json:"priority"` // P1|P2|P3
	AsA             string `json:"as_a"`
	IWant           string `json:"i_want"`
	SoThat          string `json:"so_that"`
	Narrative       string `json:"narrative,omitempty"` // prose-style body (when there's no As-a line)
	WhyPriority     string `json:"why_priority,omitempty"`
	IndependentTest string `json:"independent_test,omitempty"`
}

// Scenario ← a numbered "Given/When/Then" line under a story's Acceptance Scenarios.
type Scenario struct {
	SpecPrefix   string `json:"spec_prefix"`
	StoryOrdinal int    `json:"story_ordinal"`
	Ordinal      int    `json:"ordinal"`
	Given        string `json:"given"`
	When         string `json:"when"`
	Then         string `json:"then"`
}

// EntityRef ← a prose-derived cross-reference: an inline `[[TYPE:key]]` token, a
// converted `[label](./other.md)` link, an inline FR mention, or a Key Entities
// name. Keyed by business identity (resolved to ids at apply time); the queryable
// projection of the in-text link. (Hand-authored structured relationships are the
// separate Edge layer, not produced by import.)
type EntityRef struct {
	OwnerType  string `json:"owner_type"`  // domain|spec|requirement|user_story|entity
	OwnerKey   string `json:"owner_key"`   // business key (path|fr_key|name|abbreviation)
	TargetType string `json:"target_type"` // domain|spec|requirement|entity|milestone
	TargetKey  string `json:"target_key"`  // business key of the target
	Kind       string `json:"kind"`        // references
}

// Milestone ← a distinct milestone value referenced by the registry.
type Milestone struct {
	Abbrev string `json:"abbrev"`
}

// ---- report ----------------------------------------------------------------

// Severity classifies a Finding.
const (
	SevInfo = "info" // observation / cross-check
	SevWarn = "warn" // drift between corpus parts (registry vs spec, registry vs disk)
	SevGap  = "gap"  // source data with no home in the current ER (schema gap)
)

// Report is what a read-only parse emits: per-entity counts, the delivery-status
// histogram, and findings (drift + ER gaps) for eyeballing against the model.
type Report struct {
	Counts   map[string]int `json:"counts"`   // entity kind → row count
	Coverage map[string]int `json:"coverage"` // delivery_status → count
	Findings []Finding      `json:"findings"`
}

// Finding is one observation: a drift between corpus parts, or a piece of source
// data the current schema has nowhere to store.
type Finding struct {
	Severity string `json:"severity"` // SevInfo|SevWarn|SevGap
	Category string `json:"category"` // short tag, e.g. "spec-status", "orphan-fr"
	Message  string `json:"message"`
	Ref      string `json:"ref,omitempty"` // file/key the finding is about
}

// Add appends a finding.
func (r *Report) Add(sev, category, msg, ref string) {
	r.Findings = append(r.Findings, Finding{Severity: sev, Category: category, Message: msg, Ref: ref})
}
