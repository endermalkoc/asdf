// Package store is ASDF's entity store. Its functions operate on an Execer (a
// *sql.DB, *sql.Conn, or *sql.Tx), so the caller controls the connection and
// transaction — reads run on a pinned *sql.Conn, writes run inside the *sql.Tx
// the mutation wrapper (internal/app) owns. IDs are minted client-side: ULIDs
// for authored rows, deterministic ids for relationship rows (Edge).
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/endermalkoc/asdf/internal/ids"
)

// Execer is the subset of database/sql satisfied by *sql.DB, *sql.Conn, and
// *sql.Tx — so the same store function works for pooled reads and transactional
// writes.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ---- entities (the slice) -------------------------------------------------

type Domain struct {
	ID           string `json:"id"`
	Abbreviation string `json:"abbreviation"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Kind         string `json:"kind"`
	Status       string `json:"status"`
}

type Spec struct {
	ID       string `json:"id"`
	DomainID string `json:"domain_id"`
	Prefix   string `json:"prefix,omitempty"`
	Slug     string `json:"slug,omitempty"`
	Path     string `json:"path"`
	Title    string `json:"title,omitempty"`
	Kind     string `json:"kind"`
	Status   string `json:"status"`
	// Heading is the H1 identity line (kept on spec); all prose sections now live in
	// doc_section keyed by section_key (migration 0010).
	Heading string `json:"heading,omitempty"`
}

type Requirement struct {
	ID             string `json:"id"`
	SpecID         string `json:"spec_id"`
	Number         int    `json:"number"`
	Suffix         string `json:"suffix,omitempty"`
	FRKey          string `json:"fr_key"`
	Statement      string `json:"statement"`
	ContentStatus  string `json:"content_status"`
	DeliveryStatus string `json:"delivery_status,omitempty"`
	MilestoneID    string `json:"milestone_id,omitempty"`
	Owner          string `json:"owner,omitempty"`
	Notes          string `json:"notes,omitempty"`
	OptoutMarker   string `json:"optout_marker,omitempty"`
	OptoutReason   string `json:"optout_reason,omitempty"`
	GroupID        string `json:"group_id,omitempty"`
	Position       int    `json:"position,omitempty"`
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ---- Domain ---------------------------------------------------------------

// AddDomain mints a ULID and inserts the domain. The caller is responsible for
// committing (x is typically the mutation wrapper's *sql.Tx).
func AddDomain(ctx context.Context, x Execer, d Domain) (Domain, error) {
	if d.Kind == "" {
		d.Kind = "service"
	}
	if d.Status == "" {
		d.Status = "active"
	}
	d.ID = ids.New()
	_, err := x.ExecContext(ctx,
		"INSERT INTO `req_domain` (id,abbreviation,name,description,kind,status) VALUES (?,?,?,?,?,?)",
		d.ID, d.Abbreviation, d.Name, nullIfEmpty(d.Description), d.Kind, d.Status)
	if err != nil {
		return Domain{}, fmt.Errorf("add domain %q: %w", d.Abbreviation, err)
	}
	return d, nil
}

func ListDomains(ctx context.Context, x Execer) ([]Domain, error) {
	rows, err := x.QueryContext(ctx,
		"SELECT id,abbreviation,name,COALESCE(description,''),kind,status FROM `req_domain` ORDER BY abbreviation")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Domain
	for rows.Next() {
		var d Domain
		if err := rows.Scan(&d.ID, &d.Abbreviation, &d.Name, &d.Description, &d.Kind, &d.Status); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func domainIDByAbbrev(ctx context.Context, x Execer, abbrev string) (string, error) {
	var id string
	err := x.QueryRowContext(ctx, "SELECT id FROM `req_domain` WHERE abbreviation=?", abbrev).Scan(&id)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("no domain with abbreviation %q", abbrev)
	}
	return id, err
}

// ---- Spec -----------------------------------------------------------------

// AddSpec resolves the domain by abbreviation, mints a ULID, and inserts the spec.
func AddSpec(ctx context.Context, x Execer, domainAbbrev string, sp Spec) (Spec, error) {
	domID, err := domainIDByAbbrev(ctx, x, domainAbbrev)
	if err != nil {
		return Spec{}, err
	}
	sp.DomainID = domID
	if sp.Kind == "" {
		sp.Kind = "feature"
	}
	if sp.Status == "" {
		sp.Status = "active"
	}
	sp.ID = ids.New()
	_, err = x.ExecContext(ctx,
		"INSERT INTO `req_spec` (id,domain_id,prefix,slug,path,title,kind,status) VALUES (?,?,?,?,?,?,?,?)",
		sp.ID, sp.DomainID, nullIfEmpty(sp.Prefix), nullIfEmpty(sp.Slug), sp.Path, nullIfEmpty(sp.Title), sp.Kind, sp.Status)
	if err != nil {
		return Spec{}, fmt.Errorf("add spec %q: %w", sp.Path, err)
	}
	return sp, nil
}

func specIDByPrefix(ctx context.Context, x Execer, prefix string) (string, error) {
	var id string
	err := x.QueryRowContext(ctx, "SELECT id FROM `req_spec` WHERE prefix=?", prefix).Scan(&id)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("no spec with prefix %q", prefix)
	}
	return id, err
}

// ---- Requirement ----------------------------------------------------------

// AddRequirement resolves the spec by prefix, allocates the next sequential
// number within that spec, derives the fr_key, mints a ULID, and inserts.
//
// Number allocation locks the spec row (FOR UPDATE) so concurrent adds within
// one branch serialize; the UNIQUE(spec_id,number,suffix) constraint is the
// backstop. Cross-branch numbering convergence is the documented merge-renumber
// policy (identifiers.md), out of scope here.
func AddRequirement(ctx context.Context, x Execer, specPrefix string, r Requirement) (Requirement, error) {
	specID, err := specIDByPrefix(ctx, x, specPrefix)
	if err != nil {
		return Requirement{}, err
	}
	// Lock the spec row so concurrent number allocation serializes (no-op outside a tx).
	if _, err := x.ExecContext(ctx, "SELECT id FROM `req_spec` WHERE id=? FOR UPDATE", specID); err != nil {
		return Requirement{}, err
	}
	var maxNum sql.NullInt64
	if err := x.QueryRowContext(ctx,
		"SELECT MAX(number) FROM `req_requirement` WHERE spec_id=?", specID).Scan(&maxNum); err != nil {
		return Requirement{}, err
	}
	r.SpecID = specID
	r.Number = int(maxNum.Int64) + 1
	r.FRKey = fmt.Sprintf("%s-FR-%03d%s", specPrefix, r.Number, r.Suffix)
	if r.ContentStatus == "" {
		r.ContentStatus = "active"
	}
	r.ID = ids.New()
	now := time.Now().UTC()
	_, err = x.ExecContext(ctx,
		"INSERT INTO `req_requirement` (id,spec_id,number,suffix,fr_key,statement,content_status,delivery_status,milestone_id,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)",
		r.ID, r.SpecID, r.Number, nullIfEmpty(r.Suffix), r.FRKey, r.Statement, r.ContentStatus, nullIfEmpty(r.DeliveryStatus), nullIfEmpty(r.MilestoneID), now, now)
	if err != nil {
		return Requirement{}, fmt.Errorf("add requirement to %q: %w", specPrefix, err)
	}
	return r, nil
}

func ListRequirements(ctx context.Context, x Execer, specPrefix string) ([]Requirement, error) {
	specID, err := specIDByPrefix(ctx, x, specPrefix)
	if err != nil {
		return nil, err
	}
	rows, err := x.QueryContext(ctx,
		"SELECT id,spec_id,number,COALESCE(suffix,''),fr_key,statement,content_status,COALESCE(delivery_status,''),COALESCE(milestone_id,'') FROM `req_requirement` WHERE spec_id=? ORDER BY number",
		specID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Requirement
	for rows.Next() {
		var r Requirement
		if err := rows.Scan(&r.ID, &r.SpecID, &r.Number, &r.Suffix, &r.FRKey, &r.Statement, &r.ContentStatus, &r.DeliveryStatus, &r.MilestoneID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func reqIDByFRKey(ctx context.Context, x Execer, frKey string) (string, error) {
	var id string
	err := x.QueryRowContext(ctx, "SELECT id FROM `req_requirement` WHERE fr_key=?", frKey).Scan(&id)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("no requirement with fr_key %q", frKey)
	}
	return id, err
}

// ---- Edge (deterministic PK) ----------------------------------------------

// AddEdge links two requirements by their fr_keys. The edge's id is DERIVED
// deterministically from its identity, so re-adding the same edge is a no-op
// (INSERT IGNORE) — the merge-convergence property. Returns the deterministic id.
func AddEdge(ctx context.Context, x Execer, fromFRKey, kind, toFRKey string) (string, error) {
	fromID, err := reqIDByFRKey(ctx, x, fromFRKey)
	if err != nil {
		return "", err
	}
	toID, err := reqIDByFRKey(ctx, x, toFRKey)
	if err != nil {
		return "", err
	}
	const fromType, toType = "requirement", "requirement"
	id := ids.Rel(fromType, fromID, toType, toID, kind)
	if _, err := x.ExecContext(ctx,
		"INSERT IGNORE INTO `req_edge` (id,from_type,from_id,to_type,to_id,kind) VALUES (?,?,?,?,?,?)",
		id, fromType, fromID, toType, toID, kind); err != nil {
		return "", fmt.Errorf("add edge %s -%s-> %s: %w", fromFRKey, kind, toFRKey, err)
	}
	return id, nil
}

// SeedActor inserts the given actor if absent (idempotent on the handle), so
// changeset.author_id has a row to reference.
func SeedActor(ctx context.Context, x Execer, handle, name string) (string, error) {
	var id string
	err := x.QueryRowContext(ctx, "SELECT id FROM `rev_actor` WHERE handle=?", handle).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	id = ids.New()
	kind := "human"
	_, err = x.ExecContext(ctx,
		"INSERT INTO `rev_actor` (id,kind,name,handle) VALUES (?,?,?,?)",
		id, kind, name, handle)
	if err != nil {
		return "", fmt.Errorf("seed actor %q: %w", handle, err)
	}
	return id, nil
}
