package store

import "context"

// Read-only hygiene queries for `cusp doctor` — structural gaps that aren't integrity errors but
// are usually worth surfacing: a domain carrying no specs, or a prefixed (FR-bearing) spec with no
// requirements. Ordered for stable output; honor the branch the Execer is on.

// EmptyDomains returns the slugs of domains that own no specs.
func EmptyDomains(ctx context.Context, x Execer) ([]string, error) {
	return scanStrings(ctx, x, `
		SELECT d.slug
		FROM `+"`req_domain`"+` d
		LEFT JOIN `+"`req_spec`"+` s ON s.domain_id = d.id
		GROUP BY d.id, d.slug
		HAVING COUNT(s.id) = 0
		ORDER BY d.slug`)
}

// SpecsWithoutRequirements returns the prefixes of FR-bearing specs (those that declare a prefix)
// that have no requirements. FR-exempt specs (no prefix) are excluded — having none is expected.
func SpecsWithoutRequirements(ctx context.Context, x Execer) ([]string, error) {
	return scanStrings(ctx, x, `
		SELECT s.prefix
		FROM `+"`req_spec`"+` s
		LEFT JOIN `+"`req_requirement`"+` r ON r.spec_id = s.id
		WHERE s.prefix IS NOT NULL AND s.prefix <> ''
		GROUP BY s.id, s.prefix
		HAVING COUNT(r.id) = 0
		ORDER BY s.prefix`)
}

func scanStrings(ctx context.Context, x Execer, query string) ([]string, error) {
	rows, err := x.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
