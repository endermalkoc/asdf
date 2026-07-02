package app

import (
	"context"

	"github.com/endermalkoc/cusp/internal/storage/schema"
	"github.com/endermalkoc/cusp/internal/store"
)

// Workspace health roll-up for `cusp doctor`: it composes the existing analyses — integrity
// (Check: dangling refs + edge cycles), requirement→test-case coverage (Coverage), and a couple of
// structural hygiene checks (empty domains / FR-bearing specs with no requirements). Read-only.

// HygieneFinding is one non-error structural gap.
type HygieneFinding struct {
	Kind     string `json:"kind"`     // "empty-domain" | "empty-spec"
	Location string `json:"location"` // domain slug / spec prefix
}

// CoverageSummary is the coverage counts a doctor report carries (the full lists live in `cusp coverage`).
type CoverageSummary struct {
	Total   int `json:"total"`
	Covered int `json:"covered"`
	Orphans int `json:"orphans"`
	Drift   int `json:"drift"`
}

// SchemaStatus compares the database's applied schema version to the one this binary embeds.
// "behind" → the DB predates this cusp (pending migrations; fixable by `doctor --fix`); "ahead" →
// the DB was migrated by a newer cusp (upgrade this binary before writing — not fixable here).
type SchemaStatus struct {
	Current int    `json:"current"`
	Latest  int    `json:"latest"`
	Status  string `json:"status"` // ok | behind | ahead
	Pending int    `json:"pending,omitempty"`
}

// DoctorReport is the aggregated health picture. It is healthy when there are no integrity errors
// and the schema is in sync; coverage gaps, hygiene, and fr_key drift are informational.
type DoctorReport struct {
	Integrity  []CheckFinding     `json:"integrity"`
	Schema     SchemaStatus       `json:"schema"`
	Coverage   CoverageSummary    `json:"coverage"`
	Hygiene    []HygieneFinding   `json:"hygiene"`
	FRKeyDrift []store.FRKeyDrift `json:"frKeyDrift"`
}

// Healthy reports whether the workspace has no blocking problems: no integrity errors and a schema
// that matches this binary. (Coverage/hygiene/fr_key drift are fixable warnings, not failures.)
func (r DoctorReport) Healthy() bool { return len(r.Integrity) == 0 && r.Schema.Status == "ok" }

// Doctor runs the aggregated health checks against x.
func Doctor(ctx context.Context, x store.Execer) (DoctorReport, error) {
	integ, err := Check(ctx, x)
	if err != nil {
		return DoctorReport{}, err
	}
	sch, err := schemaStatus(ctx, x)
	if err != nil {
		return DoctorReport{}, err
	}
	cov, err := Coverage(ctx, x)
	if err != nil {
		return DoctorReport{}, err
	}
	hyg, err := hygiene(ctx, x)
	if err != nil {
		return DoctorReport{}, err
	}
	drift, err := store.DriftedFRKeys(ctx, x)
	if err != nil {
		return DoctorReport{}, err
	}
	if integ == nil {
		integ = []CheckFinding{}
	}
	if drift == nil {
		drift = []store.FRKeyDrift{}
	}
	return DoctorReport{
		Integrity: integ,
		Schema:    sch,
		Coverage: CoverageSummary{
			Total: cov.Total, Covered: cov.Covered,
			Orphans: len(cov.Orphans), Drift: len(cov.Drift),
		},
		Hygiene:    hyg,
		FRKeyDrift: drift,
	}, nil
}

func schemaStatus(ctx context.Context, x store.Execer) (SchemaStatus, error) {
	cur, err := schema.CurrentVersion(ctx, x)
	if err != nil {
		return SchemaStatus{}, err
	}
	latest := schema.LatestVersion()
	s := SchemaStatus{Current: cur, Latest: latest, Status: "ok"}
	switch {
	case cur < latest:
		s.Status = "behind"
		if pending, e := schema.PendingVersions(ctx, x); e == nil {
			s.Pending = len(pending)
		}
	case cur > latest:
		s.Status = "ahead"
	}
	return s, nil
}

func hygiene(ctx context.Context, x store.Execer) ([]HygieneFinding, error) {
	out := []HygieneFinding{}
	domains, err := store.EmptyDomains(ctx, x)
	if err != nil {
		return nil, err
	}
	for _, d := range domains {
		out = append(out, HygieneFinding{Kind: "empty-domain", Location: d})
	}
	specs, err := store.SpecsWithoutRequirements(ctx, x)
	if err != nil {
		return nil, err
	}
	for _, s := range specs {
		out = append(out, HygieneFinding{Kind: "empty-spec", Location: s})
	}
	return out, nil
}
