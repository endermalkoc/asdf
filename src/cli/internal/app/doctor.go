package app

import (
	"context"

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

// DoctorReport is the aggregated health picture. It is healthy when there are no integrity
// findings; coverage gaps and hygiene warnings are informational, not failures.
type DoctorReport struct {
	Integrity []CheckFinding   `json:"integrity"`
	Coverage  CoverageSummary  `json:"coverage"`
	Hygiene   []HygieneFinding `json:"hygiene"`
}

// Healthy reports whether the workspace has no integrity errors.
func (r DoctorReport) Healthy() bool { return len(r.Integrity) == 0 }

// Doctor runs the aggregated health checks against x.
func Doctor(ctx context.Context, x store.Execer) (DoctorReport, error) {
	integ, err := Check(ctx, x)
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
	if integ == nil {
		integ = []CheckFinding{}
	}
	return DoctorReport{
		Integrity: integ,
		Coverage: CoverageSummary{
			Total: cov.Total, Covered: cov.Covered,
			Orphans: len(cov.Orphans), Drift: len(cov.Drift),
		},
		Hygiene: hyg,
	}, nil
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
