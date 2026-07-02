package app

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/endermalkoc/cusp/internal/store"
)

// EntityDiff is one entity's change within a changeset: which entity (a polymorphic subject,
// the same coordinates a Comment anchors to), whether it was added/modified/removed, and the
// per-field base→head values. It lets a review surface anchor a comment to a concrete
// requirement/spec/entity row and field, rather than a drifting generated line.
type EntityDiff struct {
	SubjectType string      `json:"subjectType"` // requirement|spec|user_story|entity|deliverable|test_case
	SubjectID   string      `json:"subjectId"`
	SubjectRef  string      `json:"subjectRef"` // display label, e.g. "requirement:ATT-FR-012"
	DocRef      string      `json:"docRef"`     // the doc to render for a base/head diff (spec prefix); "" if none
	ChangeType  string      `json:"changeType"` // added|modified|removed
	Fields      []FieldDiff `json:"fields"`
}

// FieldDiff is one column's base→head change inside an entity.
type FieldDiff struct {
	Name string `json:"name"`
	Base string `json:"base"`
	Head string `json:"head"`
}

// diffSubjectTables maps the tables that back a Comment subject type to that type. Only these
// tables produce EntityDiffs; changes to child/junction tables show up in the table-level
// summary (workspace.Diff), not here.
var diffSubjectTables = []struct{ table, subjectType string }{
	{"req_requirement", "requirement"},
	{"req_spec", "spec"},
	{"req_user_story", "user_story"},
	{"ent_entity", "entity"},
	{"plan_deliverable", "deliverable"},
	{"test_case", "test_case"},
}

// noiseFields carry no review-meaningful content, so they are excluded from a FieldDiff even
// when they changed (id is the identity; the timestamps churn on every edit).
var noiseFields = map[string]bool{"id": true, "created_at": true, "updated_at": true}

// EntityDiffs returns the per-entity, per-field changes between base and head for the entity
// types a comment can anchor to. Runs on the pinned connection; base/head are commit refs or
// branches. Reuses the same dolt_diff plumbing as auto-generation (quoteRef).
func EntityDiffs(ctx context.Context, conn *sql.Conn, base, head string) ([]EntityDiff, error) {
	changed, err := changedTables(ctx, conn, base, head)
	if err != nil {
		return nil, err
	}
	targets, err := store.ListRefTargets(ctx, conn)
	if err != nil {
		return nil, err
	}
	label := labelFromTargets(targets)
	specRef := specRefsFromTargets(targets)
	var out []EntityDiff
	for _, st := range diffSubjectTables {
		if !changed[st.table] {
			continue
		}
		ds, err := tableEntityDiffs(ctx, conn, base, head, st.table, st.subjectType, label, specRef)
		if err != nil {
			return nil, err
		}
		out = append(out, ds...)
	}
	// Doc-coverage pass: a change confined to a spec/entity's child tables (a prose section, a
	// requirement group, a scenario) leaves the subject row untouched, so no diff above — but the
	// rendered doc still changed. Roll those up to a doc-level entry so the diff still opens.
	covered := map[string]bool{}
	for _, d := range out {
		if d.DocRef != "" {
			covered[d.DocRef] = true
		}
	}
	docs, err := docLevelDiffs(ctx, conn, base, head, changed, label, specRef, covered)
	if err != nil {
		return nil, err
	}
	out = append(out, docs...)
	// Normalize Fields to a non-nil slice so the JSON is `[]`, not `null` — a doc-level (roll-up)
	// entry has no field detail, and consumers iterate Fields directly.
	for i := range out {
		if out[i].Fields == nil {
			out[i].Fields = []FieldDiff{}
		}
	}
	return out, nil
}

// docLevelDiffs rolls a changed child/section row up to its owning spec or entity doc, emitting a
// doc-level EntityDiff (no field detail) for any doc not already covered by a subject-row diff.
// Reuses the auto-gen owner-id extraction (diffOwnerIDs / scenarioSpecIDs).
func docLevelDiffs(ctx context.Context, conn *sql.Conn, base, head string, changed map[string]bool, label func(typ, id string) string, specRef map[string]string, covered map[string]bool) ([]EntityDiff, error) {
	var out []EntityDiff
	add := func(ownerType, id string) {
		var docRef string
		switch ownerType {
		case "spec":
			if r := specRef[id]; r != "" {
				docRef = "spec:" + r
			}
		case "entity":
			if l := label("entity", id); strings.HasPrefix(l, "entity:") {
				docRef = l
			}
		}
		if docRef == "" || covered[docRef] {
			return
		}
		covered[docRef] = true
		out = append(out, EntityDiff{
			SubjectType: ownerType,
			SubjectID:   id,
			SubjectRef:  label(ownerType, id),
			DocRef:      docRef,
			ChangeType:  "modified",
		})
	}
	specChildren := []struct{ table, toCol, fromCol string }{
		{"req_spec_section", "to_spec_id", "from_spec_id"},
		{"req_requirement_group", "to_spec_id", "from_spec_id"},
	}
	for _, c := range specChildren {
		if !changed[c.table] {
			continue
		}
		ids, err := diffOwnerIDs(ctx, conn, base, head, c.table, c.toCol, c.fromCol)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			add("spec", id)
		}
	}
	if changed["req_acceptance_scenario"] {
		ids, err := scenarioSpecIDs(ctx, conn, base, head)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			add("spec", id)
		}
	}
	if changed["ent_entity_section"] {
		ids, err := diffOwnerIDs(ctx, conn, base, head, "ent_entity_section", "to_entity_id", "from_entity_id")
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			add("entity", id)
		}
	}
	// A relationship edit changes both endpoints' rendered docs (each lists its relationships).
	if changed["ent_relationship"] {
		for _, cols := range [][2]string{
			{"to_from_entity_id", "from_from_entity_id"},
			{"to_to_entity_id", "from_to_entity_id"},
		} {
			ids, err := diffOwnerIDs(ctx, conn, base, head, "ent_relationship", cols[0], cols[1])
			if err != nil {
				return nil, err
			}
			for _, id := range ids {
				add("entity", id)
			}
		}
	}
	return out, nil
}

// labelFromTargets is LabelIndex's mapping, built from an already-loaded target list: (type,id)
// → "type:key", falling back to "type:id".
func labelFromTargets(targets []store.RefTargetRow) func(typ, id string) string {
	label := make(map[string]string, len(targets))
	for _, t := range targets {
		if k := t.Type + "\x00" + t.ID; label[k] == "" {
			label[k] = t.Type + ":" + t.Key
		}
	}
	return func(typ, id string) string {
		if l, ok := label[typ+"\x00"+id]; ok {
			return l
		}
		return typ + ":" + id
	}
}

// specRefsFromTargets maps a spec id → the ref to pass to `cusp spec render` (its prefix, else
// its path). The prefix entry is listed first in ListRefTargets, so it wins.
func specRefsFromTargets(targets []store.RefTargetRow) map[string]string {
	m := make(map[string]string)
	for _, t := range targets {
		if t.Type == "spec" {
			if _, ok := m[t.ID]; !ok {
				m[t.ID] = t.Key
			}
		}
	}
	return m
}

// changedTables reports which tables differ between base and head (via dolt_diff_stat).
func changedTables(ctx context.Context, conn *sql.Conn, base, head string) (map[string]bool, error) {
	rows, err := conn.QueryContext(ctx, "SELECT table_name FROM dolt_diff_stat(?, ?)", base, head)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := map[string]bool{}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		set[t] = true
	}
	return set, rows.Err()
}

// tableEntityDiffs turns one table's row-level dolt_diff into EntityDiffs. It reads the diff
// with SELECT * and pairs the to_<col>/from_<col> columns dynamically, so it needs no static
// column list and tracks schema changes automatically.
func tableEntityDiffs(ctx context.Context, conn *sql.Conn, base, head, table, subjectType string, label func(typ, id string) string, specRef map[string]string) ([]EntityDiff, error) {
	q := fmt.Sprintf("SELECT * FROM dolt_diff(%s, %s, '%s')", quoteRef(base), quoteRef(head), table)
	rows, err := conn.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []EntityDiff
	for rows.Next() {
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]string, len(cols))
		for i, c := range cols {
			m[c] = cellString(cells[i])
		}
		subjectID := m["to_id"]
		if subjectID == "" {
			subjectID = m["from_id"]
		}
		var fields []FieldDiff
		for _, c := range cols {
			name, ok := strings.CutPrefix(c, "to_")
			if !ok || noiseFields[name] || c == "to_commit" || c == "to_commit_date" {
				continue
			}
			b, h := m["from_"+name], m["to_"+name]
			if b != h {
				fields = append(fields, FieldDiff{Name: name, Base: b, Head: h})
			}
		}
		out = append(out, EntityDiff{
			SubjectType: subjectType,
			SubjectID:   subjectID,
			SubjectRef:  label(subjectType, subjectID),
			DocRef:      docRefFor(subjectType, subjectID, m, specRef),
			ChangeType:  m["diff_type"],
			Fields:      fields,
		})
	}
	return out, rows.Err()
}

// docRefFor returns the doc to render for a base/head diff of this subject, as a self-describing
// token the client dispatches on: a requirement or user story diffs its owning spec (`spec:<ref>`,
// ref = prefix or path); a spec diffs itself; an entity diffs its own doc (`entity:<name>`, which
// `cusp entity render` resolves by name). deliverable/test_case have no single-doc render yet.
func docRefFor(subjectType, subjectID string, m map[string]string, specRef map[string]string) string {
	switch subjectType {
	case "spec":
		if r := specRef[subjectID]; r != "" {
			return "spec:" + r
		}
		return ""
	case "requirement", "user_story":
		sid := m["to_spec_id"]
		if sid == "" {
			sid = m["from_spec_id"]
		}
		if r := specRef[sid]; r != "" {
			return "spec:" + r
		}
		return ""
	case "entity":
		name := m["to_name"]
		if name == "" {
			name = m["from_name"]
		}
		if name != "" {
			return "entity:" + name
		}
		return ""
	default:
		return ""
	}
}

// cellString renders a scanned dolt_diff cell as a string ("" for NULL).
func cellString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(t)
	case string:
		return t
	case time.Time:
		return t.Format(time.RFC3339)
	default:
		return fmt.Sprintf("%v", t)
	}
}
