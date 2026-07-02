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

// ReviewVerdict — rev_review.verdict. Set by `cusp review`; a closed set (hard-validated
// via app.ValidateEnum). The verbs reference these constants instead of bare literals.
const (
	VerdictApprove        = "approve"
	VerdictDeny           = "deny"
	VerdictRequestChanges = "request_changes"
)

var ReviewVerdict = []string{VerdictApprove, VerdictDeny, VerdictRequestChanges}

// CommentSubjectType — rev_comment.subject_type (nullable). The polymorphic anchor a review
// comment can bind to; a closed set (hard-validated when present). NULL = a changeset-level
// comment with no subject. requirement/spec/entity are resolvable from a [[TYPE:key]] token
// today; user_story/test_case/deliverable are addressed by explicit type+id until they gain
// ref tokens.
var CommentSubjectType = []string{"requirement", "spec", "user_story", "test_case", "entity", "deliverable"}

// StatusForVerdict maps a review verdict to the changeset status it implies:
// approve→approved, request_changes→changes_requested, deny→denied. (merged/closed stay owned
// by the changeset merge/abandon verbs.) Returns "" for an unrecognized verdict.
func StatusForVerdict(verdict string) string {
	switch verdict {
	case VerdictApprove:
		return ChangesetApproved
	case VerdictRequestChanges:
		return ChangesetChangesRequested
	case VerdictDeny:
		return ChangesetDenied
	}
	return ""
}

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

// ---- Planning layer (closed sets) ----------------------------------------

// CapabilityLevel — plan_capability.level (nullable; validated when present).
var CapabilityLevel = []string{"domain", "epic", "capability"}

// DeliverableStatus — plan_deliverable.status. The store defaults an empty value to
// DeliverableProposed; the verbs reference these constants instead of bare literals.
const (
	DeliverableProposed = "proposed"
	DeliverableSpecced  = "specced"
	DeliverableWired    = "wired"
	DeliverableBuilt    = "built"
	DeliverableShip     = "ship"
)

var DeliverableStatus = []string{
	DeliverableProposed, DeliverableSpecced, DeliverableWired, DeliverableBuilt, DeliverableShip,
}

// DeliverableSize — plan_deliverable.size (nullable; validated when present).
var DeliverableSize = []string{"S", "M", "L", "XL"}

// DeliverableAIReady — plan_deliverable.ai_ready (nullable; validated when present).
var DeliverableAIReady = []string{"yes", "no", "na"}

// ---- Testing layer (Qase-derived closed sets) ----------------------------

// TestLayer — test_case.layer.
var TestLayer = []string{"unit", "integration", "e2e", "component", "shared"}

// TestType — test_case.type.
var TestType = []string{"functional", "smoke", "regression", "acceptance", "other"}

// TestSeverity — test_case.severity.
var TestSeverity = []string{"trivial", "minor", "normal", "major", "critical", "blocker"}

// TestAutomation — test_case.automation.
var TestAutomation = []string{"manual", "automated", "to_be_automated"}

// TestCaseStatus — test_case.status (lifecycle, not a run outcome; default draft).
var TestCaseStatus = []string{"draft", "active", "deprecated"}

// TestRunStatus — test_run.status (default active).
var TestRunStatus = []string{"active", "complete", "aborted"}

// TestResultStatus — test_result.status (run outcome).
var TestResultStatus = []string{"passed", "failed", "blocked", "skipped", "invalid", "in_progress"}

// Valid reports whether v is in allowed.
func Valid(allowed []string, v string) bool {
	for _, a := range allowed {
		if a == v {
			return true
		}
	}
	return false
}
