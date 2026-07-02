package store

import "context"

// Requirement→test-case coverage rollup, read from req_requirement_test_case. One row per
// requirement with the number of test cases linked to it, its spec prefix, and its delivery
// status (whose policy — counts_as_covered — lives in plan_delivery_status). Feeds `cusp
// coverage` (see app.Coverage); read-only, honors the branch the Execer is on.

// RequirementCoverageRow is a requirement's coverage facts.
type RequirementCoverageRow struct {
	FRKey          string `json:"frKey"`
	SpecPrefix     string `json:"specPrefix"`
	DeliveryStatus string `json:"deliveryStatus,omitempty"`
	TestCases      int    `json:"testCases"`
}

// RequirementCoverage returns every requirement with its linked-test-case count, ordered by spec
// then requirement number for stable output.
func RequirementCoverage(ctx context.Context, x Execer) ([]RequirementCoverageRow, error) {
	rows, err := x.QueryContext(ctx, `
		SELECT r.fr_key, COALESCE(s.prefix,''), COALESCE(r.delivery_status,''), COUNT(j.test_case_id)
		FROM `+"`req_requirement`"+` r
		JOIN `+"`req_spec`"+` s ON s.id = r.spec_id
		LEFT JOIN `+"`req_requirement_test_case`"+` j ON j.requirement_id = r.id
		GROUP BY r.id, r.fr_key, s.prefix, r.delivery_status, r.number
		ORDER BY s.prefix, r.number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RequirementCoverageRow
	for rows.Next() {
		var c RequirementCoverageRow
		if err := rows.Scan(&c.FRKey, &c.SpecPrefix, &c.DeliveryStatus, &c.TestCases); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
