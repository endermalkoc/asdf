// Package enums holds ASDF's allowed enum value sets. The schema stores enums as
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

// SEED sets — open value-sets with documented defaults that are project-/tenant-/
// tooling-specific. Unknown values are ACCEPTED with a warning (app.ValidateEnumSoft);
// --strict restores hard rejection. These are seed/policy, not core — see CLAUDE.md
// "Known tutor-isms to genericize".
var (
	DomainKind   = []string{"service", "shared", "infrastructure", "entities", "analysis"}
	SpecKind     = []string{"feature", "entity", "journey", "analysis", "index", "meta", "reference"}
	Priority     = []string{"P1", "P2", "P3", "P4", "P5"} // open range; corpus uses P1–P4
	OptoutMarker = []string{"none", "visual", "ops", "untestable"}
)

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
