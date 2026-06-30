package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/endermalkoc/cusp/internal/enums"
	"github.com/endermalkoc/cusp/internal/ids"
)

// This file holds idempotent, business-key upserts used by the import pipeline
// (internal/importer). Unlike the interactive Add* functions, these accept
// explicit identities (e.g. a Requirement's exact number/suffix from its fr_key,
// never auto-incremented) and converge on re-run: an existing row is found by its
// UNIQUE business key and UPDATEd; a new row gets a fresh ULID. Re-importing the
// same corpus is therefore a no-op beyond timestamps.
//
// Each function returns the row id and whether it inserted (true) or updated.

// Milestone is the importable subset of a milestone row.
type Milestone struct {
	ID     string
	Slug   string
	Name   string
	Status string
}

// UpsertDomain upserts by slug.
func UpsertDomain(ctx context.Context, x Execer, d Domain) (string, bool, error) {
	if d.Status == "" {
		d.Status = "active"
	}
	var id string
	err := x.QueryRowContext(ctx, "SELECT id FROM `req_domain` WHERE slug=?", d.Slug).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `req_domain` (id,slug,name,description,status) VALUES (?,?,?,?,?)",
			id, d.Slug, d.Name, nullIfEmpty(d.Description), d.Status)
		if err != nil {
			return "", false, fmt.Errorf("insert domain %q: %w", d.Slug, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	_, err = x.ExecContext(ctx,
		"UPDATE `req_domain` SET name=?, description=?, status=? WHERE id=?",
		d.Name, nullIfEmpty(d.Description), d.Status, id)
	if err != nil {
		return "", false, fmt.Errorf("update domain %q: %w", d.Slug, err)
	}
	return id, false, nil
}

// UpsertMilestone upserts by slug.
func UpsertMilestone(ctx context.Context, x Execer, m Milestone) (string, bool, error) {
	if m.Status == "" {
		m.Status = enums.MilestonePending
	}
	var id string
	err := x.QueryRowContext(ctx, "SELECT id FROM `plan_milestone` WHERE slug=?", m.Slug).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		now := time.Now().UTC()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `plan_milestone` (id,slug,name,status,created_at,updated_at) VALUES (?,?,?,?,?,?)",
			id, m.Slug, nullIfEmpty(m.Name), m.Status, now, now)
		if err != nil {
			return "", false, fmt.Errorf("insert milestone %q: %w", m.Slug, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	return id, false, nil
}

// UpsertSpec upserts by prefix (when set) else by path. domainID is the resolved
// owning domain.
func UpsertSpec(ctx context.Context, x Execer, domainID string, sp Spec) (string, bool, error) {
	if sp.Status == "" {
		sp.Status = "draft"
	}
	sp.DomainID = domainID

	var (
		id  string
		err error
	)
	if sp.Prefix != "" {
		err = x.QueryRowContext(ctx, "SELECT id FROM `req_spec` WHERE prefix=?", sp.Prefix).Scan(&id)
	} else {
		// prefix-less doc → identified by its location (domain + directory + filename);
		// path/slug are NULL-safe matched (path is NULL for top-level docs).
		err = x.QueryRowContext(ctx, "SELECT id FROM `req_spec` WHERE domain_id=? AND path<=>? AND slug<=>?",
			sp.DomainID, nullIfEmpty(sp.Path), nullIfEmpty(sp.Slug)).Scan(&id)
	}
	now := time.Now().UTC()
	// created_at is the source "Created" date when known. The column is NOT NULL, so a spec
	// with no source date falls back to import time on insert; an existing row keeps its
	// created_at on update (COALESCE) unless the source now carries a date.
	srcCreated := isoDateArg(sp.CreatedAt) // time.Time or nil
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		insertCreated := srcCreated
		if insertCreated == nil {
			insertCreated = now
		}
		_, err = x.ExecContext(ctx,
			"INSERT INTO `req_spec` (id,domain_id,prefix,slug,path,title,status,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?)",
			id, sp.DomainID, nullIfEmpty(sp.Prefix), nullIfEmpty(sp.Slug), nullIfEmpty(sp.Path), nullIfEmpty(sp.Title), sp.Status,
			insertCreated, now)
		if err != nil {
			return "", false, fmt.Errorf("insert spec %q: %w", sp.Path, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	// COALESCE keeps the existing created_at when the source carries no date.
	_, err = x.ExecContext(ctx,
		"UPDATE `req_spec` SET domain_id=?, slug=?, path=?, title=?, status=?, created_at=COALESCE(?, created_at), updated_at=? WHERE id=?",
		sp.DomainID, nullIfEmpty(sp.Slug), nullIfEmpty(sp.Path), nullIfEmpty(sp.Title), sp.Status,
		srcCreated, now, id)
	if err != nil {
		return "", false, fmt.Errorf("update spec %q: %w", sp.Path, err)
	}
	return id, false, nil
}

// UpsertRequirement upserts by (spec_id, number, suffix) — the explicit identity
// from the fr_key, never auto-numbered. The caller supplies specID and any
// resolved milestone_id on r.
func UpsertRequirement(ctx context.Context, x Execer, specID string, r Requirement) (string, bool, error) {
	r.SpecID = specID
	if r.ContentStatus == "" {
		r.ContentStatus = "active"
	}
	now := time.Now().UTC()

	var id string
	err := x.QueryRowContext(ctx,
		"SELECT id FROM `req_requirement` WHERE spec_id=? AND number=? AND suffix <=> ?",
		specID, r.Number, nullIfEmpty(r.Suffix)).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `req_requirement` (id,spec_id,number,suffix,fr_key,statement,content_status,delivery_status,milestone_id,notes,group_id,position,priority,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
			id, specID, r.Number, nullIfEmpty(r.Suffix), r.FRKey, nullIfEmpty(r.Statement), r.ContentStatus,
			nullIfEmpty(r.DeliveryStatus), nullIfEmpty(r.MilestoneID), nullIfEmpty(r.Notes), nullIfEmpty(r.GroupID), r.Position, nullIfNil(r.Priority), now, now)
		if err != nil {
			return "", false, fmt.Errorf("insert requirement %q: %w", r.FRKey, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	_, err = x.ExecContext(ctx,
		"UPDATE `req_requirement` SET fr_key=?, statement=?, content_status=?, delivery_status=?, milestone_id=?, notes=?, group_id=?, position=?, updated_at=? WHERE id=?",
		r.FRKey, nullIfEmpty(r.Statement), r.ContentStatus, nullIfEmpty(r.DeliveryStatus), nullIfEmpty(r.MilestoneID),
		nullIfEmpty(r.Notes), nullIfEmpty(r.GroupID), r.Position, now, id)
	if err != nil {
		return "", false, fmt.Errorf("update requirement %q: %w", r.FRKey, err)
	}
	return id, false, nil
}

// UpsertRequirementGroup upserts an FR group by (spec_id, title).
func UpsertRequirementGroup(ctx context.Context, x Execer, specID string, position int, title, notes string) (string, bool, error) {
	var id string
	err := x.QueryRowContext(ctx,
		"SELECT id FROM `req_requirement_group` WHERE spec_id=? AND title=?", specID, title).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `req_requirement_group` (id,spec_id,position,title,notes) VALUES (?,?,?,?,?)",
			id, specID, position, title, nullIfEmpty(notes))
		if err != nil {
			return "", false, fmt.Errorf("insert requirement_group %s/%q: %w", specID, title, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	_, err = x.ExecContext(ctx,
		"UPDATE `req_requirement_group` SET position=?, notes=? WHERE id=?", position, nullIfEmpty(notes), id)
	if err != nil {
		return "", false, fmt.Errorf("update requirement_group %s/%q: %w", specID, title, err)
	}
	return id, false, nil
}

// UserStory is the importable subset of a user_story row.
type UserStory struct {
	SpecID          string
	Position        int
	Title           string
	Priority        int // 0–4 (req_priority); a story always has one
	AsA             string
	IWant           string
	SoThat          string
	Narrative       string
	WhyPriority     string
	IndependentTest string
}

// UpsertUserStory upserts by (spec_id, position).
func UpsertUserStory(ctx context.Context, x Execer, us UserStory) (string, bool, error) {
	var id string
	err := x.QueryRowContext(ctx,
		"SELECT id FROM `req_user_story` WHERE spec_id=? AND position=?", us.SpecID, us.Position).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `req_user_story` (id,spec_id,position,title,priority,as_a,i_want,so_that,narrative,why_priority,independent_test) VALUES (?,?,?,?,?,?,?,?,?,?,?)",
			id, us.SpecID, us.Position, nullIfEmpty(us.Title), us.Priority, nullIfEmpty(us.AsA), nullIfEmpty(us.IWant), nullIfEmpty(us.SoThat), nullIfEmpty(us.Narrative), nullIfEmpty(us.WhyPriority), nullIfEmpty(us.IndependentTest))
		if err != nil {
			return "", false, fmt.Errorf("insert user_story %s#%d: %w", us.SpecID, us.Position, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	_, err = x.ExecContext(ctx,
		"UPDATE `req_user_story` SET title=?, priority=?, as_a=?, i_want=?, so_that=?, narrative=?, why_priority=?, independent_test=? WHERE id=?",
		nullIfEmpty(us.Title), us.Priority, nullIfEmpty(us.AsA), nullIfEmpty(us.IWant), nullIfEmpty(us.SoThat), nullIfEmpty(us.Narrative), nullIfEmpty(us.WhyPriority), nullIfEmpty(us.IndependentTest), id)
	if err != nil {
		return "", false, fmt.Errorf("update user_story %s#%d: %w", us.SpecID, us.Position, err)
	}
	return id, false, nil
}

// Scenario is the importable subset of an acceptance_scenario row.
type Scenario struct {
	UserStoryID string
	Position    int
	Given       string
	When        string
	Then        string
}

// UpsertScenario upserts by (user_story_id, position). There is no UNIQUE on that
// pair, so existence is checked explicitly.
func UpsertScenario(ctx context.Context, x Execer, s Scenario) (string, bool, error) {
	var id string
	err := x.QueryRowContext(ctx,
		"SELECT id FROM `req_acceptance_scenario` WHERE user_story_id=? AND position=?", s.UserStoryID, s.Position).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `req_acceptance_scenario` (id,user_story_id,position,`given`,`when`,`then`) VALUES (?,?,?,?,?,?)",
			id, s.UserStoryID, s.Position, nullIfEmpty(s.Given), nullIfEmpty(s.When), nullIfEmpty(s.Then))
		if err != nil {
			return "", false, fmt.Errorf("insert scenario %s#%d: %w", s.UserStoryID, s.Position, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	_, err = x.ExecContext(ctx,
		"UPDATE `req_acceptance_scenario` SET `given`=?, `when`=?, `then`=? WHERE id=?",
		nullIfEmpty(s.Given), nullIfEmpty(s.When), nullIfEmpty(s.Then), id)
	if err != nil {
		return "", false, fmt.Errorf("update scenario %s#%d: %w", s.UserStoryID, s.Position, err)
	}
	return id, false, nil
}

// UpsertExternalRef upserts by (subject_type, subject_id, system). Used to record
// a requirement's id in an outside tracker (e.g. a beads issue used as a milestone).
func UpsertExternalRef(ctx context.Context, x Execer, subjectType, subjectID, system, externalID, url string) (string, bool, error) {
	var id string
	err := x.QueryRowContext(ctx,
		"SELECT id FROM `pub_external_ref` WHERE subject_type=? AND subject_id=? AND `system`=?",
		subjectType, subjectID, system).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `pub_external_ref` (id,subject_type,subject_id,`system`,external_id,url) VALUES (?,?,?,?,?,?)",
			id, subjectType, subjectID, system, externalID, nullIfEmpty(url))
		if err != nil {
			return "", false, fmt.Errorf("insert external_ref %s/%s: %w", system, externalID, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	_, err = x.ExecContext(ctx,
		"UPDATE `pub_external_ref` SET external_id=?, url=? WHERE id=?",
		externalID, nullIfEmpty(url), id)
	if err != nil {
		return "", false, fmt.Errorf("update external_ref %s/%s: %w", system, externalID, err)
	}
	return id, false, nil
}

// Entity is the importable subset of an entity row. Its prose sections live in
// ent_entity_section (typed by ent_entity_section_type, migration 0013); description is
// the glossary one-liner (not a section).
type Entity struct {
	Name        string
	Path        string // full doc path (e.g. entities/student.md)
	Description string
	Status      string
}

// UpsertEntity upserts by name (UK). Entities are domain-less; path is the full doc path.
func UpsertEntity(ctx context.Context, x Execer, e Entity) (string, bool, error) {
	if e.Status == "" {
		e.Status = enums.EntityDraft
	}
	var id string
	err := x.QueryRowContext(ctx, "SELECT id FROM `ent_entity` WHERE name=?", e.Name).Scan(&id)
	cols := "path=?, description=?, status=?"
	args := []any{nullIfEmpty(e.Path), nullIfEmpty(e.Description), e.Status}
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `ent_entity` (id,name,path,description,status) VALUES (?,?,?,?,?)",
			append([]any{id, e.Name}, args...)...)
		if err != nil {
			return "", false, fmt.Errorf("insert entity %q: %w", e.Name, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	_, err = x.ExecContext(ctx, "UPDATE `ent_entity` SET "+cols+" WHERE id=?", append(args, id)...)
	if err != nil {
		return "", false, fmt.Errorf("update entity %q: %w", e.Name, err)
	}
	return id, false, nil
}

// upsertSection upserts one prose section by (ownerID, sectionTypeKey) — the natural
// key. Body-only; heading/level/order come from the curated type. Callers reconcile the
// owner's full set with the matching Delete*Sections* first.
func upsertSection(ctx context.Context, x Execer, sectionTable, ownerCol, ownerID, sectionTypeKey, body string) (string, bool, error) {
	var id string
	err := x.QueryRowContext(ctx,
		"SELECT id FROM `"+sectionTable+"` WHERE "+ownerCol+"=? AND section_type_slug=?",
		ownerID, sectionTypeKey).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `"+sectionTable+"` (id,"+ownerCol+",section_type_slug,body) VALUES (?,?,?,?)",
			id, ownerID, sectionTypeKey, nullIfEmpty(body))
		if err != nil {
			return "", false, fmt.Errorf("insert %s %s/%s: %w", sectionTable, ownerID, sectionTypeKey, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	_, err = x.ExecContext(ctx, "UPDATE `"+sectionTable+"` SET body=? WHERE id=?", nullIfEmpty(body), id)
	if err != nil {
		return "", false, fmt.Errorf("update %s %s/%s: %w", sectionTable, ownerID, sectionTypeKey, err)
	}
	return id, false, nil
}

func deleteSectionsByOwner(ctx context.Context, x Execer, sectionTable, ownerCol, ownerID string) error {
	if _, err := x.ExecContext(ctx,
		"DELETE FROM `"+sectionTable+"` WHERE "+ownerCol+"=?", ownerID); err != nil {
		return fmt.Errorf("delete sections from %s for %s: %w", sectionTable, ownerID, err)
	}
	return nil
}

// UpsertSpecSection / UpsertEntitySection write one prose section (body) under a spec /
// entity, addressed by its curated section-type key.
func UpsertSpecSection(ctx context.Context, x Execer, specID, sectionTypeKey, body string) (string, bool, error) {
	return upsertSection(ctx, x, "req_spec_section", "spec_id", specID, sectionTypeKey, body)
}

// UpsertEntityRelationship records an entity↔entity relationship (from the Drizzle
// schema) with a deterministic id over its identity, so re-import is idempotent and
// merge-safe (INSERT IGNORE).
func UpsertEntityRelationship(ctx context.Context, x Execer, fromID, toID, cardinality, junctionTable string) (string, error) {
	id := ids.Rel(fromID, toID, cardinality, junctionTable)
	if _, err := x.ExecContext(ctx,
		"INSERT IGNORE INTO `ent_relationship` (id,from_entity_id,to_entity_id,cardinality,junction_table) VALUES (?,?,?,?,?)",
		id, fromID, toID, nullIfEmpty(cardinality), nullIfEmpty(junctionTable)); err != nil {
		return "", fmt.Errorf("add entity_relationship %s -> %s: %w", fromID, toID, err)
	}
	return id, nil
}

func UpsertEntitySection(ctx context.Context, x Execer, entityID, sectionTypeKey, body string) (string, bool, error) {
	return upsertSection(ctx, x, "ent_entity_section", "entity_id", entityID, sectionTypeKey, body)
}

// DeleteSpecSectionsBySpec / DeleteEntitySectionsByEntity remove an owner's sections —
// the first step of per-owner reconciliation, so a re-import leaves no orphans.
func DeleteSpecSectionsBySpec(ctx context.Context, x Execer, specID string) error {
	return deleteSectionsByOwner(ctx, x, "req_spec_section", "spec_id", specID)
}
func DeleteEntitySectionsByEntity(ctx context.Context, x Execer, entityID string) error {
	return deleteSectionsByOwner(ctx, x, "ent_entity_section", "entity_id", entityID)
}

// upsertSectionType inserts/updates one curated section-type row (the lookup vocabulary).
// Used by the `section-type add` CLI; the importer never writes types (they are seeded).
func upsertSectionType(ctx context.Context, x Execer, typeTable string, t SectionTypeRow) (bool, error) {
	if t.Origin == "" {
		t.Origin = "authored"
	}
	var existing string
	err := x.QueryRowContext(ctx, "SELECT slug FROM `"+typeTable+"` WHERE slug=?", t.Key).Scan(&existing)
	args := []any{nullIfEmpty(t.Title), t.Level, t.Position, nullIfEmpty(t.Description), t.Origin}
	switch {
	case err == sql.ErrNoRows:
		_, err = x.ExecContext(ctx,
			"INSERT INTO `"+typeTable+"` (slug,title,level,position,description,origin) VALUES (?,?,?,?,?,?)",
			append([]any{t.Key}, args...)...)
		if err != nil {
			return false, fmt.Errorf("insert section type %q: %w", t.Key, err)
		}
		return true, nil
	case err != nil:
		return false, err
	}
	_, err = x.ExecContext(ctx,
		"UPDATE `"+typeTable+"` SET title=?, level=?, position=?, description=?, origin=? WHERE slug=?",
		append(args, t.Key)...)
	if err != nil {
		return false, fmt.Errorf("update section type %q: %w", t.Key, err)
	}
	return false, nil
}

// UpsertSpecSectionType / UpsertEntitySectionType add (or update) a curated section type.
func UpsertSpecSectionType(ctx context.Context, x Execer, t SectionTypeRow) (bool, error) {
	return upsertSectionType(ctx, x, "req_spec_section_type", t)
}
func UpsertEntitySectionType(ctx context.Context, x Execer, t SectionTypeRow) (bool, error) {
	return upsertSectionType(ctx, x, "ent_entity_section_type", t)
}

// AddEdgeByIDs links two nodes by resolved ids with a deterministic edge id
// (idempotent via INSERT IGNORE). Unlike AddEdge it does not resolve fr_keys, so
// the import can link any node types it has already inserted.
func AddEdgeByIDs(ctx context.Context, x Execer, fromType, fromID, kind, toType, toID string) (string, error) {
	id := ids.Rel(fromType, fromID, toType, toID, kind)
	if _, err := x.ExecContext(ctx,
		"INSERT IGNORE INTO `req_edge` (id,from_type,from_id,to_type,to_id,kind) VALUES (?,?,?,?,?,?)",
		id, fromType, fromID, toType, toID, kind); err != nil {
		return "", fmt.Errorf("add edge %s -%s-> %s: %w", fromID, kind, toID, err)
	}
	return id, nil
}

// UpsertEntityRef records a prose-derived cross-reference (the queryable form of
// an inline [[TYPE:key]] link) by resolved ids, with a deterministic id over its
// UNIQUE identity (idempotent via INSERT IGNORE). Callers reconcile a owner's full
// set with DeleteEntityRefsByOwner before re-inserting its current refs.
func UpsertEntityRef(ctx context.Context, x Execer, ownerType, ownerID, targetType, targetID string) (string, error) {
	id := ids.Rel(ownerType, ownerID, targetType, targetID)
	if _, err := x.ExecContext(ctx,
		"INSERT IGNORE INTO `req_entity_ref` (id,owner_type,owner_id,target_type,target_id) VALUES (?,?,?,?,?)",
		id, ownerType, ownerID, targetType, targetID); err != nil {
		return "", fmt.Errorf("add entity_ref %s -> %s: %w", ownerID, targetID, err)
	}
	return id, nil
}

// DeleteEntityRefsByOwner removes every entity_ref owned by one node — the first
// step of per-owner reconciliation, so a token removed from the owner's text drops
// its row (the deterministic id makes a re-added token converge, not duplicate).
func DeleteEntityRefsByOwner(ctx context.Context, x Execer, ownerType, ownerID string) error {
	if _, err := x.ExecContext(ctx,
		"DELETE FROM `req_entity_ref` WHERE owner_type=? AND owner_id=?",
		ownerType, ownerID); err != nil {
		return fmt.Errorf("delete entity_refs for %s %s: %w", ownerType, ownerID, err)
	}
	return nil
}
