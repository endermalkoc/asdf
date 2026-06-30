// Package store is Cusp's entity store. Its functions operate on an Execer (a
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

	"github.com/endermalkoc/cusp/internal/enums"
	"github.com/endermalkoc/cusp/internal/ids"
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
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
}

type Spec struct {
	ID        string `json:"id"`
	DomainID  string `json:"domain_id"`
	Prefix    string `json:"prefix,omitempty"`
	Slug      string `json:"slug,omitempty"`
	Path      string `json:"path"`
	Title     string `json:"title,omitempty"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at,omitempty"` // source "Created" date (YYYY-MM-DD); "" → unknown (NULL)
	// Title is the single spec label; the H1 renders as `# {title}`. All prose
	// sections live in req_spec_section, each typed by req_spec_section_type (0013).
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
	Notes          string `json:"notes,omitempty"`
	GroupID        string `json:"group_id,omitempty"`
	Position       int    `json:"position,omitempty"`
	Priority       *int   `json:"priority,omitempty"` // 0–4 (req_priority); nil = unprioritized
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// isoDateArg parses a YYYY-MM-DD date into a SQL arg (UTC midnight), or NULL when the string
// is empty or unparseable — so an unknown source date is stored as NULL, not fabricated.
func isoDateArg(s string) any {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return t.UTC()
}

// nullIfNil maps a nil *int to SQL NULL, else its value (for nullable int columns).
func nullIfNil(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// ---- Domain ---------------------------------------------------------------

// AddDomain mints a ULID and inserts the domain. The caller is responsible for
// committing (x is typically the mutation wrapper's *sql.Tx).
func AddDomain(ctx context.Context, x Execer, d Domain) (Domain, error) {
	if d.Status == "" {
		d.Status = "active"
	}
	d.ID = ids.New()
	_, err := x.ExecContext(ctx,
		"INSERT INTO `req_domain` (id,slug,name,description,status) VALUES (?,?,?,?,?)",
		d.ID, d.Slug, d.Name, nullIfEmpty(d.Description), d.Status)
	if err != nil {
		return Domain{}, fmt.Errorf("add domain %q: %w", d.Slug, err)
	}
	return d, nil
}

func ListDomains(ctx context.Context, x Execer) ([]Domain, error) {
	rows, err := x.QueryContext(ctx,
		"SELECT id,slug,name,COALESCE(description,''),status FROM `req_domain` ORDER BY slug")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Domain
	for rows.Next() {
		var d Domain
		if err := rows.Scan(&d.ID, &d.Slug, &d.Name, &d.Description, &d.Status); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func domainIDBySlug(ctx context.Context, x Execer, slug string) (string, error) {
	var id string
	err := x.QueryRowContext(ctx, "SELECT id FROM `req_domain` WHERE slug=?", slug).Scan(&id)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("no domain with slug %q", slug)
	}
	return id, err
}

// ---- Spec -----------------------------------------------------------------

// AddSpec resolves the domain by slug, mints a ULID, and inserts the spec.
func AddSpec(ctx context.Context, x Execer, domainSlug string, sp Spec) (Spec, error) {
	domID, err := domainIDBySlug(ctx, x, domainSlug)
	if err != nil {
		return Spec{}, err
	}
	sp.DomainID = domID
	if sp.Status == "" {
		sp.Status = "active"
	}
	sp.ID = ids.New()
	_, err = x.ExecContext(ctx,
		"INSERT INTO `req_spec` (id,domain_id,prefix,slug,path,title,status) VALUES (?,?,?,?,?,?,?)",
		sp.ID, sp.DomainID, nullIfEmpty(sp.Prefix), nullIfEmpty(sp.Slug), nullIfEmpty(sp.Path), nullIfEmpty(sp.Title), sp.Status)
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
		"INSERT INTO `req_requirement` (id,spec_id,number,suffix,fr_key,statement,content_status,delivery_status,milestone_id,priority,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)",
		r.ID, r.SpecID, r.Number, nullIfEmpty(r.Suffix), r.FRKey, r.Statement, r.ContentStatus, nullIfEmpty(r.DeliveryStatus), nullIfEmpty(r.MilestoneID), nullIfNil(r.Priority), now, now)
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
		"SELECT id,spec_id,number,COALESCE(suffix,''),fr_key,COALESCE(statement,''),content_status,COALESCE(delivery_status,''),COALESCE(milestone_id,'') FROM `req_requirement` WHERE spec_id=? ORDER BY number",
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

// Edges (the deterministic-PK cross-reference graph) are written via AddEdgeByIDs
// (import.go) from resolved (type,id) endpoints; see cmd/cusp/edge.go + app/edge.go for
// endpoint resolution and the acyclicity check.

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
	kind := enums.ActorHuman
	_, err = x.ExecContext(ctx,
		"INSERT INTO `rev_actor` (id,kind,name,handle) VALUES (?,?,?,?)",
		id, kind, name, handle)
	if err != nil {
		return "", fmt.Errorf("seed actor %q: %w", handle, err)
	}
	return id, nil
}
