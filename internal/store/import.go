package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/endermalkoc/asdf/internal/ids"
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
	ID           string
	Abbreviation string
	Name         string
	Status       string
}

// UpsertDomain upserts by abbreviation.
func UpsertDomain(ctx context.Context, x Execer, d Domain) (string, bool, error) {
	if d.Kind == "" {
		d.Kind = "service"
	}
	if d.Status == "" {
		d.Status = "active"
	}
	var id string
	err := x.QueryRowContext(ctx, "SELECT id FROM `domain` WHERE abbreviation=?", d.Abbreviation).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `domain` (id,abbreviation,name,description,kind,status) VALUES (?,?,?,?,?,?)",
			id, d.Abbreviation, d.Name, nullIfEmpty(d.Description), d.Kind, d.Status)
		if err != nil {
			return "", false, fmt.Errorf("insert domain %q: %w", d.Abbreviation, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	_, err = x.ExecContext(ctx,
		"UPDATE `domain` SET name=?, description=?, kind=?, status=? WHERE id=?",
		d.Name, nullIfEmpty(d.Description), d.Kind, d.Status, id)
	if err != nil {
		return "", false, fmt.Errorf("update domain %q: %w", d.Abbreviation, err)
	}
	return id, false, nil
}

// UpsertMilestone upserts by abbreviation.
func UpsertMilestone(ctx context.Context, x Execer, m Milestone) (string, bool, error) {
	if m.Status == "" {
		m.Status = "pending"
	}
	var id string
	err := x.QueryRowContext(ctx, "SELECT id FROM `milestone` WHERE abbreviation=?", m.Abbreviation).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		now := time.Now().UTC()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `milestone` (id,abbreviation,name,status,created_at,updated_at) VALUES (?,?,?,?,?,?)",
			id, m.Abbreviation, nullIfEmpty(m.Name), m.Status, now, now)
		if err != nil {
			return "", false, fmt.Errorf("insert milestone %q: %w", m.Abbreviation, err)
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
	if sp.Kind == "" {
		sp.Kind = "feature"
	}
	if sp.Status == "" {
		sp.Status = "draft"
	}
	sp.DomainID = domainID

	var (
		id  string
		err error
	)
	if sp.Prefix != "" {
		err = x.QueryRowContext(ctx, "SELECT id FROM `spec` WHERE prefix=?", sp.Prefix).Scan(&id)
	} else {
		err = x.QueryRowContext(ctx, "SELECT id FROM `spec` WHERE path=?", sp.Path).Scan(&id)
	}
	now := time.Now().UTC()
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `spec` (id,domain_id,prefix,slug,path,title,kind,status,heading,preamble,overview,edge_cases,success_criteria,platform_scope,assumptions,clarifications,more_info,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
			id, sp.DomainID, nullIfEmpty(sp.Prefix), nullIfEmpty(sp.Slug), sp.Path, nullIfEmpty(sp.Title), sp.Kind, sp.Status,
			nullIfEmpty(sp.Heading), nullIfEmpty(sp.Preamble), nullIfEmpty(sp.Overview), nullIfEmpty(sp.EdgeCases),
			nullIfEmpty(sp.SuccessCriteria), nullIfEmpty(sp.PlatformScope), nullIfEmpty(sp.Assumptions), nullIfEmpty(sp.Clarifications), nullIfEmpty(sp.MoreInfo), now, now)
		if err != nil {
			return "", false, fmt.Errorf("insert spec %q: %w", sp.Path, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	_, err = x.ExecContext(ctx,
		"UPDATE `spec` SET domain_id=?, slug=?, path=?, title=?, kind=?, status=?, heading=?, preamble=?, overview=?, edge_cases=?, success_criteria=?, platform_scope=?, assumptions=?, clarifications=?, more_info=?, updated_at=? WHERE id=?",
		sp.DomainID, nullIfEmpty(sp.Slug), sp.Path, nullIfEmpty(sp.Title), sp.Kind, sp.Status,
		nullIfEmpty(sp.Heading), nullIfEmpty(sp.Preamble), nullIfEmpty(sp.Overview), nullIfEmpty(sp.EdgeCases),
		nullIfEmpty(sp.SuccessCriteria), nullIfEmpty(sp.PlatformScope), nullIfEmpty(sp.Assumptions), nullIfEmpty(sp.Clarifications), nullIfEmpty(sp.MoreInfo), now, id)
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
	if r.OptoutMarker == "" {
		r.OptoutMarker = "none"
	}
	now := time.Now().UTC()

	var id string
	err := x.QueryRowContext(ctx,
		"SELECT id FROM `requirement` WHERE spec_id=? AND number=? AND suffix <=> ?",
		specID, r.Number, nullIfEmpty(r.Suffix)).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `requirement` (id,spec_id,number,suffix,fr_key,statement,content_status,delivery_status,milestone_id,owner,notes,optout_marker,group_id,position,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
			id, specID, r.Number, nullIfEmpty(r.Suffix), r.FRKey, nullIfEmpty(r.Statement), r.ContentStatus,
			nullIfEmpty(r.DeliveryStatus), nullIfEmpty(r.MilestoneID), nullIfEmpty(r.Owner), nullIfEmpty(r.Notes), r.OptoutMarker, nullIfEmpty(r.GroupID), r.Position, now, now)
		if err != nil {
			return "", false, fmt.Errorf("insert requirement %q: %w", r.FRKey, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	_, err = x.ExecContext(ctx,
		"UPDATE `requirement` SET fr_key=?, statement=?, content_status=?, delivery_status=?, milestone_id=?, owner=?, notes=?, optout_marker=?, group_id=?, position=?, updated_at=? WHERE id=?",
		r.FRKey, nullIfEmpty(r.Statement), r.ContentStatus, nullIfEmpty(r.DeliveryStatus), nullIfEmpty(r.MilestoneID),
		nullIfEmpty(r.Owner), nullIfEmpty(r.Notes), r.OptoutMarker, nullIfEmpty(r.GroupID), r.Position, now, id)
	if err != nil {
		return "", false, fmt.Errorf("update requirement %q: %w", r.FRKey, err)
	}
	return id, false, nil
}

// UpsertRequirementGroup upserts an FR group by (spec_id, header).
func UpsertRequirementGroup(ctx context.Context, x Execer, specID string, position int, header, note string) (string, bool, error) {
	var id string
	err := x.QueryRowContext(ctx,
		"SELECT id FROM `requirement_group` WHERE spec_id=? AND header=?", specID, header).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `requirement_group` (id,spec_id,position,header,note) VALUES (?,?,?,?,?)",
			id, specID, position, header, nullIfEmpty(note))
		if err != nil {
			return "", false, fmt.Errorf("insert requirement_group %s/%q: %w", specID, header, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	_, err = x.ExecContext(ctx,
		"UPDATE `requirement_group` SET position=?, note=? WHERE id=?", position, nullIfEmpty(note), id)
	if err != nil {
		return "", false, fmt.Errorf("update requirement_group %s/%q: %w", specID, header, err)
	}
	return id, false, nil
}

// UserStory is the importable subset of a user_story row.
type UserStory struct {
	SpecID          string
	Ordinal         int
	Title           string
	Priority        string
	AsA             string
	IWant           string
	SoThat          string
	Narrative       string
	WhyPriority     string
	IndependentTest string
}

// UpsertUserStory upserts by (spec_id, ordinal).
func UpsertUserStory(ctx context.Context, x Execer, us UserStory) (string, bool, error) {
	var id string
	err := x.QueryRowContext(ctx,
		"SELECT id FROM `user_story` WHERE spec_id=? AND ordinal=?", us.SpecID, us.Ordinal).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `user_story` (id,spec_id,ordinal,title,priority,as_a,i_want,so_that,narrative,why_priority,independent_test) VALUES (?,?,?,?,?,?,?,?,?,?,?)",
			id, us.SpecID, us.Ordinal, nullIfEmpty(us.Title), nullIfEmpty(us.Priority), nullIfEmpty(us.AsA), nullIfEmpty(us.IWant), nullIfEmpty(us.SoThat), nullIfEmpty(us.Narrative), nullIfEmpty(us.WhyPriority), nullIfEmpty(us.IndependentTest))
		if err != nil {
			return "", false, fmt.Errorf("insert user_story %s#%d: %w", us.SpecID, us.Ordinal, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	_, err = x.ExecContext(ctx,
		"UPDATE `user_story` SET title=?, priority=?, as_a=?, i_want=?, so_that=?, narrative=?, why_priority=?, independent_test=? WHERE id=?",
		nullIfEmpty(us.Title), nullIfEmpty(us.Priority), nullIfEmpty(us.AsA), nullIfEmpty(us.IWant), nullIfEmpty(us.SoThat), nullIfEmpty(us.Narrative), nullIfEmpty(us.WhyPriority), nullIfEmpty(us.IndependentTest), id)
	if err != nil {
		return "", false, fmt.Errorf("update user_story %s#%d: %w", us.SpecID, us.Ordinal, err)
	}
	return id, false, nil
}

// Scenario is the importable subset of an acceptance_scenario row.
type Scenario struct {
	UserStoryID string
	Ordinal     int
	Given       string
	When        string
	Then        string
}

// UpsertScenario upserts by (user_story_id, ordinal). There is no UNIQUE on that
// pair, so existence is checked explicitly.
func UpsertScenario(ctx context.Context, x Execer, s Scenario) (string, bool, error) {
	var id string
	err := x.QueryRowContext(ctx,
		"SELECT id FROM `acceptance_scenario` WHERE user_story_id=? AND ordinal=?", s.UserStoryID, s.Ordinal).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `acceptance_scenario` (id,user_story_id,ordinal,`given`,`when`,`then`) VALUES (?,?,?,?,?,?)",
			id, s.UserStoryID, s.Ordinal, nullIfEmpty(s.Given), nullIfEmpty(s.When), nullIfEmpty(s.Then))
		if err != nil {
			return "", false, fmt.Errorf("insert scenario %s#%d: %w", s.UserStoryID, s.Ordinal, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	_, err = x.ExecContext(ctx,
		"UPDATE `acceptance_scenario` SET `given`=?, `when`=?, `then`=? WHERE id=?",
		nullIfEmpty(s.Given), nullIfEmpty(s.When), nullIfEmpty(s.Then), id)
	if err != nil {
		return "", false, fmt.Errorf("update scenario %s#%d: %w", s.UserStoryID, s.Ordinal, err)
	}
	return id, false, nil
}

// UpsertExternalRef upserts by (subject_type, subject_id, system). Used to record
// a requirement's id in an outside tracker (e.g. a beads issue used as a milestone).
func UpsertExternalRef(ctx context.Context, x Execer, subjectType, subjectID, system, externalID, url string) (string, bool, error) {
	var id string
	err := x.QueryRowContext(ctx,
		"SELECT id FROM `external_ref` WHERE subject_type=? AND subject_id=? AND `system`=?",
		subjectType, subjectID, system).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `external_ref` (id,subject_type,subject_id,`system`,external_id,url) VALUES (?,?,?,?,?,?)",
			id, subjectType, subjectID, system, externalID, nullIfEmpty(url))
		if err != nil {
			return "", false, fmt.Errorf("insert external_ref %s/%s: %w", system, externalID, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	_, err = x.ExecContext(ctx,
		"UPDATE `external_ref` SET external_id=?, url=? WHERE id=?",
		externalID, nullIfEmpty(url), id)
	if err != nil {
		return "", false, fmt.Errorf("update external_ref %s/%s: %w", system, externalID, err)
	}
	return id, false, nil
}

// Entity is the importable subset of an entity row.
type Entity struct {
	Name        string
	Description string
	Status      string
	// Entity-doc template sections (0003).
	Purpose         string
	KeyConcepts     string
	SchemaReference string
	Relationships   string
	BusinessRules   string
	Validations     string
	RowLevelAccess  string
	Notes           string
	SpecReferences  string
}

// UpsertEntity upserts by name (UK). specID is the documenting kind=entity spec
// (may be empty → NULL).
func UpsertEntity(ctx context.Context, x Execer, domainID, specID string, e Entity) (string, bool, error) {
	if e.Status == "" {
		e.Status = "draft"
	}
	var id string
	err := x.QueryRowContext(ctx, "SELECT id FROM `entity` WHERE name=?", e.Name).Scan(&id)
	cols := "domain_id=?, spec_id=?, description=?, status=?, purpose=?, key_concepts=?, schema_reference=?, relationships=?, business_rules=?, validations=?, row_level_access=?, entity_notes=?, spec_references=?"
	args := []any{domainID, nullIfEmpty(specID), nullIfEmpty(e.Description), e.Status,
		nullIfEmpty(e.Purpose), nullIfEmpty(e.KeyConcepts), nullIfEmpty(e.SchemaReference), nullIfEmpty(e.Relationships),
		nullIfEmpty(e.BusinessRules), nullIfEmpty(e.Validations), nullIfEmpty(e.RowLevelAccess), nullIfEmpty(e.Notes), nullIfEmpty(e.SpecReferences)}
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `entity` (id,name,domain_id,spec_id,description,status,purpose,key_concepts,schema_reference,relationships,business_rules,validations,row_level_access,entity_notes,spec_references) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
			append([]any{id, e.Name}, args...)...)
		if err != nil {
			return "", false, fmt.Errorf("insert entity %q: %w", e.Name, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	_, err = x.ExecContext(ctx, "UPDATE `entity` SET "+cols+" WHERE id=?", append(args, id)...)
	if err != nil {
		return "", false, fmt.Errorf("update entity %q: %w", e.Name, err)
	}
	return id, false, nil
}

// UpsertDocSection upserts a generic catch-all section by (owner_type, owner_id, ordinal).
func UpsertDocSection(ctx context.Context, x Execer, ownerType, ownerID string, ordinal, level int, heading, body string) (string, bool, error) {
	var id string
	err := x.QueryRowContext(ctx,
		"SELECT id FROM `doc_section` WHERE owner_type=? AND owner_id=? AND ordinal=?",
		ownerType, ownerID, ordinal).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `doc_section` (id,owner_type,owner_id,ordinal,level,heading,body) VALUES (?,?,?,?,?,?,?)",
			id, ownerType, ownerID, ordinal, level, nullIfEmpty(heading), nullIfEmpty(body))
		if err != nil {
			return "", false, fmt.Errorf("insert doc_section %s/%s#%d: %w", ownerType, ownerID, ordinal, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	_, err = x.ExecContext(ctx,
		"UPDATE `doc_section` SET level=?, heading=?, body=? WHERE id=?",
		level, nullIfEmpty(heading), nullIfEmpty(body), id)
	if err != nil {
		return "", false, fmt.Errorf("update doc_section %s/%s#%d: %w", ownerType, ownerID, ordinal, err)
	}
	return id, false, nil
}

// UpsertPrivilege upserts by (resource, scope, action) — the privilege's UNIQUE
// identity. `scope` is a reserved word in some engines, hence backticked.
func UpsertPrivilege(ctx context.Context, x Execer, resource, scope, action string) (string, bool, error) {
	var id string
	err := x.QueryRowContext(ctx,
		"SELECT id FROM `privilege` WHERE resource=? AND `scope`=? AND `action`=?",
		resource, scope, action).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		id = ids.New()
		_, err = x.ExecContext(ctx,
			"INSERT INTO `privilege` (id,resource,`scope`,`action`) VALUES (?,?,?,?)",
			id, resource, scope, action)
		if err != nil {
			return "", false, fmt.Errorf("insert privilege (%s,%s,%s): %w", resource, scope, action, err)
		}
		return id, true, nil
	case err != nil:
		return "", false, err
	}
	return id, false, nil
}

// AddEdgeByIDs links two nodes by resolved ids with a deterministic edge id
// (idempotent via INSERT IGNORE). Unlike AddEdge it does not resolve fr_keys, so
// the import can link any node types it has already inserted.
func AddEdgeByIDs(ctx context.Context, x Execer, fromType, fromID, kind, toType, toID string) (string, error) {
	id := ids.Rel(fromType, fromID, toType, toID, kind)
	if _, err := x.ExecContext(ctx,
		"INSERT IGNORE INTO `edge` (id,from_type,from_id,to_type,to_id,kind) VALUES (?,?,?,?,?,?)",
		id, fromType, fromID, toType, toID, kind); err != nil {
		return "", fmt.Errorf("add edge %s -%s-> %s: %w", fromID, kind, toID, err)
	}
	return id, nil
}

// UpsertEntityRef records a prose-derived cross-reference (the queryable form of
// an inline [[TYPE:key]] link) by resolved ids, with a deterministic id over its
// UNIQUE identity (idempotent via INSERT IGNORE). Callers reconcile a owner's full
// set with DeleteEntityRefsByOwner before re-inserting its current refs.
func UpsertEntityRef(ctx context.Context, x Execer, ownerType, ownerID, targetType, targetID, kind string) (string, error) {
	id := ids.Rel(ownerType, ownerID, targetType, targetID, kind)
	if _, err := x.ExecContext(ctx,
		"INSERT IGNORE INTO `entity_ref` (id,owner_type,owner_id,target_type,target_id,kind) VALUES (?,?,?,?,?,?)",
		id, ownerType, ownerID, targetType, targetID, kind); err != nil {
		return "", fmt.Errorf("add entity_ref %s -%s-> %s: %w", ownerID, kind, targetID, err)
	}
	return id, nil
}

// DeleteEntityRefsByOwner removes every entity_ref owned by one node — the first
// step of per-owner reconciliation, so a token removed from the owner's text drops
// its row (the deterministic id makes a re-added token converge, not duplicate).
func DeleteEntityRefsByOwner(ctx context.Context, x Execer, ownerType, ownerID string) error {
	if _, err := x.ExecContext(ctx,
		"DELETE FROM `entity_ref` WHERE owner_type=? AND owner_id=?",
		ownerType, ownerID); err != nil {
		return fmt.Errorf("delete entity_refs for %s %s: %w", ownerType, ownerID, err)
	}
	return nil
}
