package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/endermalkoc/cusp/internal/ids"
)

// GlossaryTerm is the writable shape of a glossary_term row plus its aliases.
type GlossaryTerm struct {
	ID         string
	Slug       string
	Term       string
	Definition string
	DomainID   string
	Status     string
	Aliases    []string
}

// UpsertGlossaryTerm upserts by slug (the term's UNIQUE identity / link key). The
// caller sets aliases separately via SetGlossaryAliases.
func UpsertGlossaryTerm(ctx context.Context, x Execer, t GlossaryTerm) (string, bool, error) {
	if t.Status == "" {
		t.Status = "draft"
	}
	var id string
	err := x.QueryRowContext(ctx, "SELECT id FROM `req_glossary_term` WHERE slug=?", t.Slug).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		now := time.Now().UTC()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `req_glossary_term` (id,slug,term,definition,domain_id,status,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?)",
			id, t.Slug, nullIfEmpty(t.Term), nullIfEmpty(t.Definition), nullIfEmpty(t.DomainID), t.Status, now, now)
		if err != nil {
			return "", false, fmt.Errorf("insert glossary_term %q: %w", t.Slug, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	_, err = x.ExecContext(ctx,
		"UPDATE `req_glossary_term` SET term=?, definition=?, domain_id=?, status=?, updated_at=? WHERE id=?",
		nullIfEmpty(t.Term), nullIfEmpty(t.Definition), nullIfEmpty(t.DomainID), t.Status, time.Now().UTC(), id)
	if err != nil {
		return "", false, fmt.Errorf("update glossary_term %q: %w", t.Slug, err)
	}
	return id, false, nil
}

// SetGlossaryAliases replaces a term's aliases (delete-then-insert). An alias is
// globally UNIQUE, so each is first detached from any other term (steal), then
// re-pointed here — with a deterministic id so it converges on merge.
func SetGlossaryAliases(ctx context.Context, x Execer, termID string, aliases []string) error {
	if _, err := x.ExecContext(ctx, "DELETE FROM `req_glossary_alias` WHERE term_id=?", termID); err != nil {
		return fmt.Errorf("clear glossary aliases: %w", err)
	}
	seen := map[string]bool{}
	for _, a := range aliases {
		a = strings.TrimSpace(a)
		if a == "" || seen[a] {
			continue
		}
		seen[a] = true
		if _, err := x.ExecContext(ctx, "DELETE FROM `req_glossary_alias` WHERE alias=?", a); err != nil {
			return fmt.Errorf("detach alias %q: %w", a, err)
		}
		if _, err := x.ExecContext(ctx,
			"INSERT INTO `req_glossary_alias` (id,term_id,alias) VALUES (?,?,?)",
			ids.Rel("glossary-alias", a), termID, a); err != nil {
			return fmt.Errorf("insert alias %q: %w", a, err)
		}
	}
	return nil
}

// DomainIDBySlug looks up a domain's id by its slug (ok=false if absent).
func DomainIDBySlug(ctx context.Context, x Execer, slug string) (string, bool, error) {
	var id string
	err := x.QueryRowContext(ctx, "SELECT id FROM `req_domain` WHERE slug=?", slug).Scan(&id)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}

// GlossaryTermRow is a glossary term read for rendering (with its domain + aliases).
type GlossaryTermRow struct {
	ID         string
	Slug       string
	Term       string
	Definition string
	DomainSlug string
	Status     string
	Aliases    []string
}

// ListGlossaryTerms returns every glossary term ordered by slug, each with its aliases.
func ListGlossaryTerms(ctx context.Context, x Execer) ([]GlossaryTermRow, error) {
	rows, err := x.QueryContext(ctx, `
		SELECT t.id, t.slug, COALESCE(t.term,''), COALESCE(t.definition,''),
		       COALESCE(d.slug,''), t.status
		FROM `+"`req_glossary_term`"+` t LEFT JOIN `+"`req_domain`"+` d ON t.domain_id = d.id
		ORDER BY t.slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GlossaryTermRow
	byID := map[string]int{}
	for rows.Next() {
		var t GlossaryTermRow
		if err := rows.Scan(&t.ID, &t.Slug, &t.Term, &t.Definition, &t.DomainSlug, &t.Status); err != nil {
			return nil, err
		}
		byID[t.ID] = len(out)
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	arows, err := x.QueryContext(ctx, "SELECT term_id, alias FROM `req_glossary_alias` ORDER BY alias")
	if err != nil {
		return nil, err
	}
	defer arows.Close()
	for arows.Next() {
		var termID, alias string
		if err := arows.Scan(&termID, &alias); err != nil {
			return nil, err
		}
		if i, ok := byID[termID]; ok {
			out[i].Aliases = append(out[i].Aliases, alias)
		}
	}
	return out, arows.Err()
}
