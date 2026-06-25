// Package enums holds ASDF's allowed enum value sets. The schema stores enums as
// VARCHAR (by design — keeps the sets configurable, see 0001_init.up.sql), so the
// application validates against these sets. They mirror docs/entities/enums.md.
package enums

// Value sets used by the current CLI slice. Extend as commands grow.
var (
	DomainKind          = []string{"service", "shared", "infrastructure", "entities", "analysis"}
	DomainStatus        = []string{"draft", "active", "deprecated"}
	SpecKind            = []string{"feature", "entity", "journey", "analysis", "index", "meta", "reference"}
	SpecStatus          = []string{"draft", "reviewed", "active", "obsolete"}
	ContentStatus       = []string{"draft", "active", "obsolete"}
	RequirementDelivery = []string{"covered", "test-pending", "not-implemented", "e2e-sufficient", "shared", "schema-only", "deferred"}
	EdgeKind            = []string{"references", "refines", "depends_on", "supersedes", "relates", "defers_to"}
	GlossaryStatus      = []string{"draft", "active", "deprecated"}
)

// Valid reports whether v is in allowed.
func Valid(allowed []string, v string) bool {
	for _, a := range allowed {
		if a == v {
			return true
		}
	}
	return false
}
