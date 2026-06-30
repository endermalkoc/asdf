// Package enums holds Cusp's allowed enum value sets. The schema stores enums as
// VARCHAR (by design — keeps the sets configurable, see 0001_init.up.sql), so the
// application validates against these sets. They mirror docs/entities/enums.md.
package enums

// Enum policy buckets (see docs/entities/decisions.md). All enums are VARCHAR in the
// schema; these sets are how the app validates them.

// HARD sets — closed lifecycle/workflow + structural discriminators. Unknown values
// are rejected (app.ValidateEnum). Changing one of these is a schema-level decision.
var (
	DomainStatus   = []string{"draft", "active", "deprecated"}
	SpecStatus     = []string{"draft", "reviewed", "active", "obsolete"}
	ContentStatus  = []string{"draft", "active", "obsolete"}
	EdgeKind       = []string{"references", "refines", "depends_on", "supersedes", "relates", "defers_to"}
	GlossaryStatus = []string{"draft", "active", "deprecated"}
)

// EntityStatus — ent_entity.status lifecycle. The importer seeds new entities at
// EntityDraft; promotion is a future CLI write path (the set is here so it validates
// the moment that path exists, matching DomainStatus/SpecStatus).
const (
	EntityDraft      = "draft"
	EntityActive     = "active"
	EntityDeprecated = "deprecated"
)

var EntityStatus = []string{EntityDraft, EntityActive, EntityDeprecated}

// MilestoneStatus — plan_milestone.status lifecycle. The importer seeds milestones at
// MilestonePending.
const (
	MilestoneComplete   = "complete"
	MilestoneInProgress = "in_progress"
	MilestonePending    = "pending"
)

var MilestoneStatus = []string{MilestoneComplete, MilestoneInProgress, MilestonePending}

// ChangesetStatus — rev_changeset.status. A real state machine driven by the changeset
// verbs: draft (start) → open (submit) → merged (merge) | closed (abandon); review may
// record changes_requested/approved/denied between open and merge. The verbs reference
// these constants instead of bare string literals.
const (
	ChangesetDraft            = "draft"
	ChangesetOpen             = "open"
	ChangesetChangesRequested = "changes_requested"
	ChangesetApproved         = "approved"
	ChangesetDenied           = "denied"
	ChangesetMerged           = "merged"
	ChangesetClosed           = "closed"
)

var ChangesetStatus = []string{
	ChangesetDraft, ChangesetOpen, ChangesetChangesRequested,
	ChangesetApproved, ChangesetDenied, ChangesetMerged, ChangesetClosed,
}

// ActorKind — rev_actor.kind. SeedActor writes ActorHuman; agent actors carry ActorAgent.
const (
	ActorHuman = "human"
	ActorAgent = "agent"
)

var ActorKind = []string{ActorHuman, ActorAgent}

// ReviewVerdict — rev_review.verdict. Defined for documentation/parity; the review CLI
// that would set it is not built yet (see ROADMAP), so nothing references it today.
var ReviewVerdict = []string{"approve", "deny", "request_changes"}

// SEED set — an open value-set with documented defaults, validated leniently
// (app.ValidateEnumSoft accepts unknowns with a warning; --strict rejects). Seed/policy,
// not core — see CLAUDE.md "Known tutor-isms to genericize".
//
// RequirementDelivery is the seed reference list for delivery_status — what the
// importer and the soft write-path validator check against. The authoritative,
// policy-carrying form is the `delivery_status` lookup table (migration 0009).
var RequirementDelivery = []string{
	"covered", "test-pending", "not-implemented",
	"e2e-sufficient", "shared", "schema-only", "deferred",
}

// Valid reports whether v is in allowed.
func Valid(allowed []string, v string) bool {
	for _, a := range allowed {
		if a == v {
			return true
		}
	}
	return false
}
