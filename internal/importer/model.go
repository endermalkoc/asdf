// Package importer holds the source-agnostic core of Cusp's import pipeline:
// the staging Graph (a corpus parsed into Cusp's entity shapes, keyed by
// business identifiers rather than minted DB ids) and the Report a read-only
// parse produces. Source adapters (e.g. internal/importer/tutor) populate a
// Graph; a later write pass resolves the business keys to rows through the
// store/Mutate command contract.
//
// This first cut is parse-and-report only: nothing here touches the database.
package importer

// Graph is a source corpus parsed into Cusp's entity shapes. Rows are keyed by
// business identifiers (domain slug, spec prefix, fr_key, …) because at
// parse time no ULIDs or foreign keys exist yet.
type Graph struct {
	Domains       []Domain             `json:"domains"`
	Specs         []Spec               `json:"specs"`
	Reqs          []Requirement        `json:"requirements"`
	Stories       []UserStory          `json:"user_stories"`
	Scenarios     []Scenario           `json:"acceptance_scenarios"`
	Refs          []EntityRef          `json:"entity_refs"`
	Milestones    []Milestone          `json:"milestones"`
	Entities      []Entity             `json:"entities"`
	Relationships []EntityRelationship `json:"entity_relationships,omitempty"`

	// Planning layer (populated by e.g. internal/importer/notion).
	Capabilities []Capability  `json:"capabilities,omitempty"`
	Deliverables []Deliverable `json:"deliverables,omitempty"`
	Views        []View        `json:"views,omitempty"`
}

// ---- planning layer --------------------------------------------------------
//
// Planning rows (Capability → Deliverable → View) come from an external task
// system (e.g. Notion). They carry no natural unique business column, so each
// records its source system's stable id (SourceID, e.g. a Notion page id);
// the apply pass derives a deterministic row id from it, so re-import converges
// instead of duplicating. Relations are kept as lists of source ids and resolved
// to row ids (and reconciled per owner) at apply time.

// Capability ← a row of the planning "capabilities" set (a 3-tier hierarchy via
// Level / ParentSourceID).
type Capability struct {
	SourceID             string   `json:"source_id"`
	SourceURL            string   `json:"source_url,omitempty"`
	Title                string   `json:"title"`
	Level                string   `json:"level,omitempty"` // domain|epic|capability
	DomainSlug           string   `json:"domain_slug"`
	ParentSourceID       string   `json:"parent_source_id,omitempty"`
	MilestoneSlugs       []string `json:"milestone_slugs,omitempty"`        // → capability_milestone
	DeliverableSourceIDs []string `json:"deliverable_source_ids,omitempty"` // → capability_deliverable
}

// Deliverable ← a row of the planning "deliverables" set (the external-task subject).
type Deliverable struct {
	SourceID            string   `json:"source_id"`
	SourceURL           string   `json:"source_url,omitempty"`
	Title               string   `json:"title"`
	Size                string   `json:"size,omitempty"`     // S|M|L|XL
	Status              string   `json:"status,omitempty"`   // proposed|specced|wired|built|ship
	AIReady             string   `json:"ai_ready,omitempty"` // yes|no|na
	MilestoneSlug       string   `json:"milestone_slug,omitempty"`
	CapabilitySourceIDs []string `json:"capability_source_ids,omitempty"` // → capability_deliverable
	ViewSourceIDs       []string `json:"view_source_ids,omitempty"`       // → deliverable_view
	BlockedBySourceIDs  []string `json:"blocked_by_source_ids,omitempty"` // → deliverable_dependency
	BeadIDs             string   `json:"bead_ids,omitempty"`              // raw → ExternalRef(system=beads)
}

// View ← a row of the planning "views" set (a UI surface; the bridge into specs).
type View struct {
	SourceID             string   `json:"source_id"`
	SourceURL            string   `json:"source_url,omitempty"`
	Title                string   `json:"title"`
	Route                string   `json:"route,omitempty"`
	DomainSlug           string   `json:"domain_slug"`
	SpecFile             string   `json:"spec_file,omitempty"`              // best-effort → view.spec_id
	DeliverableSourceIDs []string `json:"deliverable_source_ids,omitempty"` // → deliverable_view
}

// EntityRelationship ← a foreign-key / junction relationship extracted from the
// Drizzle schema. From/To are entity names (resolved to ids at apply time);
// Cardinality is one_to_one | one_to_many | many_to_many; JunctionTable is the
// join table for a many-to-many.
type EntityRelationship struct {
	FromName      string `json:"from_name"`
	ToName        string `json:"to_name"`
	Cardinality   string `json:"cardinality"`
	JunctionTable string `json:"junction_table,omitempty"`
}

// Entity ← a row of specs/entities/index.md (the entity glossary). A first-class,
// domain-less document; DocPath is its full doc location (stored as ent_entity.path).
type Entity struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`   // mapped to entity.status (draft|active|deprecated)
	DocPath     string `json:"doc_path"` // entities/<file>.md

	// Entity-doc prose → ent_entity_section, each addressed by a curated section key
	// (purpose, key_concepts, …); unrecognized headings fold into `notes`.
	Sections []DocSection `json:"sections,omitempty"`
}

// Domain ← a top-level directory under specs/, described in specs/index.md.
type Domain struct {
	Slug        string `json:"slug"`        // directory name, e.g. "enrollment"
	Name        string `json:"name"`        // title-cased label
	Description string `json:"description"` // from index.md → domain.description (added in 0002)
}

// Spec ← a row of specs/prefix-registry.md, enriched from the file's frontmatter
// and its document sections (for an information-complete round-trip).
type Spec struct {
	Prefix    string `json:"prefix"`     // e.g. ADDS (empty for FR-exempt docs)
	Path      string `json:"path"`       // relative to specs/, e.g. enrollment/add-student.md
	Title     string `json:"title"`      // frontmatter title
	Domain    string `json:"domain"`     // domain slug (registry column)
	RawStatus string `json:"raw_status"` // source status verbatim: Draft|Reviewed|Active
	Status    string `json:"status"`     // mapped to Cusp spec.status (draft|active|obsolete)
	Created   string `json:"created"`    // source "Created" date (YYYY-MM-DD) → spec.created_at; "" if absent

	// Prose sections → req_spec_section, each addressed by a curated section key
	// (overview, edge_cases, success_criteria, scope, assumptions, open_questions,
	// preamble, more_info); unrecognized headings fold into `notes`. The H1 is
	// rendered from Title (no separate heading is stored).
	KeyEntities []string     `json:"key_entities,omitempty"` // entity names → spec→entity refs
	Sections    []DocSection `json:"sections,omitempty"`
	ReqGroups   []ReqGroup   `json:"req_groups,omitempty"` // FR group sub-headers + notes
}

// ReqGroup is a bold FR sub-header that organizes a spec's FR list, with the
// interspersed prose (e.g. a "> See [shared/X]" blockquote) under it. Title is the
// sub-header text (stored as requirement_group.title).
type ReqGroup struct {
	Position int    `json:"position"`
	Title    string `json:"title"`
	Notes    string `json:"notes,omitempty"`
}

// DocSection is one prose section bound for the curated section model: Key is its
// section-type key (overview, edge_cases, purpose, notes, …) and Body its Markdown.
// Heading/level/order are not carried — they come from the section type on render
// (migration 0013). One DocSection per distinct key per owner (the importer merges
// collisions, notably the `notes` fold target).
type DocSection struct {
	Key  string `json:"key"`
	Body string `json:"body"`
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
	E2ERef         string `json:"e2e_ref,omitempty"`  // registry e2e_ref → folded into notes (test linkage)
	Section        string `json:"section,omitempty"`  // FR group header it belongs to (link key)
	Position       int    `json:"position,omitempty"` // document order within the spec's FR list
	Notes          string `json:"notes"`              // registry notes
	Tombstoned     bool   `json:"tombstoned"`         // spec line marks an intentional omission
}

// UserStory ← a "### User Story N - Title (Priority: PN)" heading + its As-a line.
type UserStory struct {
	SpecPrefix      string `json:"spec_prefix"`
	Position        int    `json:"position"`
	Title           string `json:"title"`
	Priority        int    `json:"priority"` // 0–4 (req_priority level); corpus P1–P4 → 1–4
	AsA             string `json:"as_a"`
	IWant           string `json:"i_want"`
	SoThat          string `json:"so_that"`
	Narrative       string `json:"narrative,omitempty"` // prose-style body (when there's no As-a line)
	WhyPriority     string `json:"why_priority,omitempty"`
	IndependentTest string `json:"independent_test,omitempty"`
}

// Scenario ← a numbered "Given/When/Then" line under a story's Acceptance Scenarios.
type Scenario struct {
	SpecPrefix    string `json:"spec_prefix"`
	StoryPosition int    `json:"story_position"`
	Position      int    `json:"position"`
	Given         string `json:"given"`
	When          string `json:"when"`
	Then          string `json:"then"`
}

// EntityRef ← a prose-derived cross-reference: an inline `[[TYPE:key]]` token, a
// converted `[label](./other.md)` link, an inline FR mention, or a Key Entities
// name. Keyed by business identity (resolved to ids at apply time); the queryable
// projection of the in-text link. (Hand-authored structured relationships are the
// separate Edge layer, not produced by import.)
type EntityRef struct {
	OwnerType  string `json:"owner_type"`  // domain|spec|requirement|user_story|entity
	OwnerKey   string `json:"owner_key"`   // business key (path|fr_key|name|slug)
	TargetType string `json:"target_type"` // domain|spec|requirement|entity|milestone
	TargetKey  string `json:"target_key"`  // business key of the target
}

// Milestone ← a distinct milestone value referenced by the registry.
type Milestone struct {
	Slug string `json:"slug"`
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
