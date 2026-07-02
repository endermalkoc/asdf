package app

import (
	"context"

	"github.com/endermalkoc/cusp/internal/store"
)

// Requirement→test-case coverage analysis for `cusp coverage`: how many requirements have at
// least one linked test case, rolled up per spec, plus the two actionable gap lists — orphan FRs
// (no test case at all) and delivery-status drift (a status whose policy counts_as_covered, yet no
// test case backs it). Read-only; honors the branch the Execer is on.

// CoverageReq is one requirement in a coverage report.
type CoverageReq struct {
	FRKey          string `json:"frKey"`
	SpecPrefix     string `json:"specPrefix,omitempty"`
	DeliveryStatus string `json:"deliveryStatus,omitempty"`
	TestCases      int    `json:"testCases"`
}

// SpecCoverage rolls up coverage for one spec.
type SpecCoverage struct {
	Prefix  string `json:"prefix"`
	Total   int    `json:"total"`
	Covered int    `json:"covered"` // requirements with >= 1 linked test case
}

// CoverageReport is the whole requirement→test-case coverage picture.
type CoverageReport struct {
	Total   int            `json:"total"`
	Covered int            `json:"covered"`
	BySpec  []SpecCoverage `json:"bySpec"`
	Orphans []CoverageReq  `json:"orphans"` // no linked test case
	Drift   []CoverageReq  `json:"drift"`   // delivery_status counts_as_covered, but no test case
}

// Coverage builds the requirement→test-case coverage report.
func Coverage(ctx context.Context, x store.Execer) (CoverageReport, error) {
	rows, err := store.RequirementCoverage(ctx, x)
	if err != nil {
		return CoverageReport{}, err
	}
	statuses, err := store.ListDeliveryStatuses(ctx, x)
	if err != nil {
		return CoverageReport{}, err
	}
	claimsCovered := make(map[string]bool, len(statuses))
	for _, s := range statuses {
		claimsCovered[s.Key] = s.CountsAsCovered
	}

	rep := CoverageReport{BySpec: []SpecCoverage{}, Orphans: []CoverageReq{}, Drift: []CoverageReq{}}
	bySpec := map[string]*SpecCoverage{}
	var order []string
	for _, r := range rows {
		rep.Total++
		covered := r.TestCases > 0
		if covered {
			rep.Covered++
		}
		sc, ok := bySpec[r.SpecPrefix]
		if !ok {
			sc = &SpecCoverage{Prefix: r.SpecPrefix}
			bySpec[r.SpecPrefix] = sc
			order = append(order, r.SpecPrefix)
		}
		sc.Total++
		if covered {
			sc.Covered++
			continue
		}
		cr := CoverageReq{FRKey: r.FRKey, SpecPrefix: r.SpecPrefix, DeliveryStatus: r.DeliveryStatus, TestCases: r.TestCases}
		rep.Orphans = append(rep.Orphans, cr)
		if claimsCovered[r.DeliveryStatus] {
			rep.Drift = append(rep.Drift, cr)
		}
	}
	for _, p := range order {
		rep.BySpec = append(rep.BySpec, *bySpec[p])
	}
	return rep, nil
}
