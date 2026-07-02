package store

import (
	"context"
	"database/sql"
)

// External-ref reads and deletes for the `cusp ref` CRUD verbs. The write path
// (idempotent upsert by (subject_type, subject_id, system)) is UpsertExternalRef
// in import.go, shared with the import pipeline. An external ref links a Cusp
// subject (deliverable | requirement | test_result) to its id/key in an outside
// system (jira, github, beads, …); the subject is polymorphic and carries no FK.

// ExternalRefRow is one external-system reference. SubjectRef is a display label
// filled in by the command layer (via app.LabelIndex), not stored.
type ExternalRefRow struct {
	ID          string `json:"id"`
	SubjectType string `json:"subjectType"`
	SubjectID   string `json:"subjectId"`
	SubjectRef  string `json:"subjectRef,omitempty"`
	System      string `json:"system"`
	ExternalID  string `json:"externalId"`
	URL         string `json:"url,omitempty"`
}

const externalRefCols = "id, subject_type, subject_id, `system`, external_id, COALESCE(url,'')"

type rowScanner interface{ Scan(...any) error }

func scanExternalRef(s rowScanner) (ExternalRefRow, error) {
	var r ExternalRefRow
	err := s.Scan(&r.ID, &r.SubjectType, &r.SubjectID, &r.System, &r.ExternalID, &r.URL)
	return r, err
}

// ListExternalRefs lists external refs, optionally filtered to a single subject
// (pass both subjectType and subjectID to filter; empty lists all). Ordered by
// subject then system for stable output.
func ListExternalRefs(ctx context.Context, x Execer, subjectType, subjectID string) ([]ExternalRefRow, error) {
	q := "SELECT " + externalRefCols + " FROM `pub_external_ref`"
	var args []any
	if subjectType != "" && subjectID != "" {
		q += " WHERE subject_type=? AND subject_id=?"
		args = append(args, subjectType, subjectID)
	}
	q += " ORDER BY subject_type, subject_id, `system`"
	rows, err := x.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExternalRefRow
	for rows.Next() {
		r, err := scanExternalRef(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetExternalRef fetches one external ref by id.
func GetExternalRef(ctx context.Context, x Execer, id string) (ExternalRefRow, bool, error) {
	r, err := scanExternalRef(x.QueryRowContext(ctx, "SELECT "+externalRefCols+" FROM `pub_external_ref` WHERE id=?", id))
	if err == sql.ErrNoRows {
		return ExternalRefRow{}, false, nil
	}
	if err != nil {
		return ExternalRefRow{}, false, err
	}
	return r, true, nil
}

// DeleteExternalRef removes one external ref by id; false if it did not exist.
func DeleteExternalRef(ctx context.Context, x Execer, id string) (bool, error) {
	res, err := x.ExecContext(ctx, "DELETE FROM `pub_external_ref` WHERE id=?", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
