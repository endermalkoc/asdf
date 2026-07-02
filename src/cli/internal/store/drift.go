package store

import (
	"context"
	"fmt"
)

// Data-drift detection/repair for the app-maintained fr_key column. An fr_key is derived from its
// spec's prefix and the requirement's number+suffix (see AddRequirement: "<prefix>-FR-<NNN><suffix>");
// it can only drift via an import bug or a manual edit, since the prefix is immutable through the
// CLI. DriftedFRKeys finds the mismatches; RecomputeFRKey writes the derived value back.

// FRKeyDrift is one requirement whose stored fr_key differs from its derived value.
type FRKeyDrift struct {
	RequirementID string `json:"-"`
	Stored        string `json:"stored"`
	Derived       string `json:"derived"`
}

// deriveFRKey builds the canonical fr_key from its parts (must match AddRequirement).
func deriveFRKey(prefix string, number int, suffix string) string {
	return fmt.Sprintf("%s-FR-%03d%s", prefix, number, suffix)
}

// DriftedFRKeys returns requirements whose stored fr_key doesn't match its derived value.
func DriftedFRKeys(ctx context.Context, x Execer) ([]FRKeyDrift, error) {
	rows, err := x.QueryContext(ctx, `
		SELECT r.id, r.fr_key, COALESCE(s.prefix,''), r.number, COALESCE(r.suffix,'')
		FROM `+"`req_requirement`"+` r
		JOIN `+"`req_spec`"+` s ON s.id = r.spec_id
		ORDER BY s.prefix, r.number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FRKeyDrift
	for rows.Next() {
		var id, stored, prefix, suffix string
		var number int
		if err := rows.Scan(&id, &stored, &prefix, &number, &suffix); err != nil {
			return nil, err
		}
		if derived := deriveFRKey(prefix, number, suffix); derived != stored {
			out = append(out, FRKeyDrift{RequirementID: id, Stored: stored, Derived: derived})
		}
	}
	return out, rows.Err()
}

// RecomputeFRKey writes the derived fr_key back for one requirement.
func RecomputeFRKey(ctx context.Context, x Execer, requirementID, derived string) error {
	if _, err := x.ExecContext(ctx, "UPDATE `req_requirement` SET fr_key=? WHERE id=?", derived, requirementID); err != nil {
		return fmt.Errorf("recompute fr_key for %s: %w", requirementID, err)
	}
	return nil
}
