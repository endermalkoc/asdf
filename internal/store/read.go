package store

import (
	"context"
	"database/sql"
)

// This file holds read queries used by the generator (internal/generate) to
// reconstruct documents from the canonical database. Plain reads, no transaction.

// SpecRow is a spec joined to its domain abbreviation. Prose sections live in
// doc_section (keyed by section_key); only the H1 heading stays on the spec.
type SpecRow struct {
	ID           string `json:"id"`
	DomainAbbrev string `json:"domain"`
	Prefix       string `json:"prefix,omitempty"`
	Path         string `json:"path"`
	Title        string `json:"title,omitempty"`
	Kind         string `json:"kind"`
	Status       string `json:"status"`
	Heading      string `json:"heading,omitempty"`
}

// ListSpecs returns every spec with its domain abbreviation, by path.
func ListSpecs(ctx context.Context, x Execer) ([]SpecRow, error) {
	rows, err := x.QueryContext(ctx, `
		SELECT s.id, d.abbreviation, COALESCE(s.prefix,''), s.path, COALESCE(s.title,''), s.kind, s.status, COALESCE(s.heading,'')
		FROM `+"`req_spec`"+` s JOIN `+"`req_domain`"+` d ON s.domain_id = d.id
		ORDER BY s.path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpecRow
	for rows.Next() {
		var s SpecRow
		if err := rows.Scan(&s.ID, &s.DomainAbbrev, &s.Prefix, &s.Path, &s.Title, &s.Kind, &s.Status, &s.Heading); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// StoryRow is a user story with the id needed to fetch its scenarios.
type StoryRow struct {
	ID              string
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

// ListStoriesBySpec returns a spec's user stories ordered by heading number.
func ListStoriesBySpec(ctx context.Context, x Execer, specID string) ([]StoryRow, error) {
	rows, err := x.QueryContext(ctx, `
		SELECT id, ordinal, COALESCE(title,''), COALESCE(priority,''), COALESCE(as_a,''), COALESCE(i_want,''),
		       COALESCE(so_that,''), COALESCE(narrative,''), COALESCE(why_priority,''), COALESCE(independent_test,'')
		FROM `+"`req_user_story`"+` WHERE spec_id=? ORDER BY ordinal`, specID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoryRow
	for rows.Next() {
		var s StoryRow
		if err := rows.Scan(&s.ID, &s.Ordinal, &s.Title, &s.Priority, &s.AsA, &s.IWant, &s.SoThat, &s.Narrative, &s.WhyPriority, &s.IndependentTest); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ScenarioRow is one acceptance scenario.
type ScenarioRow struct {
	Ordinal int
	Given   string
	When    string
	Then    string
}

// ListScenariosByStory returns a story's acceptance scenarios in order.
func ListScenariosByStory(ctx context.Context, x Execer, storyID string) ([]ScenarioRow, error) {
	rows, err := x.QueryContext(ctx, `
		SELECT ordinal, COALESCE(`+"`given`"+`,''), COALESCE(`+"`when`"+`,''), COALESCE(`+"`then`"+`,'')
		FROM `+"`req_acceptance_scenario`"+` WHERE user_story_id=? ORDER BY ordinal`, storyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScenarioRow
	for rows.Next() {
		var s ScenarioRow
		if err := rows.Scan(&s.Ordinal, &s.Given, &s.When, &s.Then); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ReqRow is the renderable subset of a requirement.
type ReqRow struct {
	FRKey          string
	Number         int
	Suffix         string
	GroupID        string
	Position       int
	Statement      string
	DeliveryStatus string
	Milestone      string
}

// ListReqsBySpecID returns a spec's requirements in document order (position;
// unpositioned registry-only FRs sort last by number), with the milestone
// abbreviation resolved (empty when none).
func ListReqsBySpecID(ctx context.Context, x Execer, specID string) ([]ReqRow, error) {
	rows, err := x.QueryContext(ctx, `
		SELECT r.fr_key, r.number, COALESCE(r.suffix,''), COALESCE(r.group_id,''), COALESCE(r.position,0), COALESCE(r.statement,''),
		       COALESCE(r.delivery_status,''), COALESCE(m.abbreviation,'')
		FROM `+"`req_requirement`"+` r
		LEFT JOIN `+"`plan_milestone`"+` m ON r.milestone_id = m.id
		WHERE r.spec_id=? ORDER BY (r.position = 0 OR r.position IS NULL), r.position, r.number, r.suffix`, specID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReqRow
	for rows.Next() {
		var r ReqRow
		if err := rows.Scan(&r.FRKey, &r.Number, &r.Suffix, &r.GroupID, &r.Position, &r.Statement, &r.DeliveryStatus, &r.Milestone); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReqGroupRow is an FR group header + note.
type ReqGroupRow struct {
	ID       string
	Position int
	Header   string
	Note     string
}

// ListReqGroups returns a spec's FR groups ordered by position.
func ListReqGroups(ctx context.Context, x Execer, specID string) ([]ReqGroupRow, error) {
	rows, err := x.QueryContext(ctx, `
		SELECT id, position, header, COALESCE(note,'') FROM `+"`req_requirement_group`"+`
		WHERE spec_id=? ORDER BY position`, specID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReqGroupRow
	for rows.Next() {
		var g ReqGroupRow
		if err := rows.Scan(&g.ID, &g.Position, &g.Header, &g.Note); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// EntityRow is the renderable subset of an entity, with its documenting spec path
// and template sections.
type EntityRow struct {
	ID          string
	Name        string
	Description string
	Status      string
	DocPath     string
}

// ListEntities returns entities ordered by name, with the path of their entity doc.
func ListEntities(ctx context.Context, x Execer) ([]EntityRow, error) {
	rows, err := x.QueryContext(ctx, `
		SELECT e.id, e.name, COALESCE(e.description,''), e.status, COALESCE(s.path,'')
		FROM `+"`ent_entity`"+` e LEFT JOIN `+"`req_spec`"+` s ON e.spec_id = s.id
		ORDER BY e.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EntityRow
	for rows.Next() {
		var e EntityRow
		if err := rows.Scan(&e.ID, &e.Name, &e.Description, &e.Status, &e.DocPath); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DocSectionRow is one document section: a recognized one carries SectionKey
// (overview, edge_cases, …); a bespoke one has SectionKey == "".
type DocSectionRow struct {
	Ordinal    int
	Level      int
	Heading    string
	Body       string
	SectionKey string
}

// ListDocSections returns an owner's sections in original order.
func ListDocSections(ctx context.Context, x Execer, ownerType, ownerID string) ([]DocSectionRow, error) {
	rows, err := x.QueryContext(ctx, `
		SELECT ordinal, level, COALESCE(heading,''), COALESCE(body,''), COALESCE(section_key,'')
		FROM `+"`req_doc_section`"+` WHERE owner_type=? AND owner_id=? ORDER BY ordinal`, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DocSectionRow
	for rows.Next() {
		var d DocSectionRow
		if err := rows.Scan(&d.Ordinal, &d.Level, &d.Heading, &d.Body, &d.SectionKey); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListKeyEntities returns the names of entities a spec links to via spec→entity refs.
func ListKeyEntities(ctx context.Context, x Execer, specID string) ([]string, error) {
	rows, err := x.QueryContext(ctx, `
		SELECT e.name FROM `+"`req_entity_ref`"+` g JOIN `+"`ent_entity`"+` e ON g.target_id = e.id
		WHERE g.owner_type='spec' AND g.owner_id=? AND g.target_type='entity'
		ORDER BY e.name`, specID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// RefTargetRow is one resolvable cross-reference target: a (type, key) the resolver
// maps to an entity id and — when the entity has a generated page — its doc path and
// in-page anchor. `req_domain`/`plan_milestone` resolve (so an entity_ref can be recorded) but
// have no page yet, so DocPath is empty and they render label-only.
type RefTargetRow struct {
	Type    string // domain|spec|requirement|entity|milestone
	Key     string // the [[TYPE:key]] key: abbreviation | prefix-or-path | fr_key | name
	ID      string
	DocPath string // generated page path; requirement = owning spec's path; "" when none
	Anchor  string // in-page anchor (requirement = fr-key slug); "" otherwise
}

// ListRefTargets returns every resolvable cross-reference target. Specs appear twice
// — keyed by prefix (when set) and by path — so `[[SPEC:ATT]]` and
// `[[SPEC:scheduling/events/take-attendance.md]]` both resolve.
func ListRefTargets(ctx context.Context, x Execer) ([]RefTargetRow, error) {
	var out []RefTargetRow
	queries := []string{
		"SELECT 'domain', abbreviation, id, '', '' FROM `req_domain`",
		"SELECT 'spec', prefix, id, path, '' FROM `req_spec` WHERE prefix IS NOT NULL AND prefix<>''",
		"SELECT 'spec', path, id, path, '' FROM `req_spec`",
		"SELECT 'requirement', r.fr_key, r.id, COALESCE(s.path,''), LOWER(r.fr_key) " +
			"FROM `req_requirement` r JOIN `req_spec` s ON r.spec_id = s.id WHERE r.fr_key IS NOT NULL AND r.fr_key<>''",
		"SELECT 'entity', e.name, e.id, COALESCE(s.path,''), '' " +
			"FROM `ent_entity` e LEFT JOIN `req_spec` s ON e.spec_id = s.id",
		"SELECT 'milestone', abbreviation, id, '', '' FROM `plan_milestone`",
		// Glossary terms resolve by slug and by alias; both link to glossary.md#slug.
		"SELECT 'glossary_term', slug, id, 'glossary.md', slug FROM `req_glossary_term`",
		"SELECT 'glossary_term', a.alias, t.id, 'glossary.md', t.slug " +
			"FROM `req_glossary_alias` a JOIN `req_glossary_term` t ON a.term_id = t.id",
	}
	for _, q := range queries {
		if err := scanRefTargets(ctx, x, &out, q); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func scanRefTargets(ctx context.Context, x Execer, out *[]RefTargetRow, query string) error {
	rows, err := x.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var t RefTargetRow
		if err := rows.Scan(&t.Type, &t.Key, &t.ID, &t.DocPath, &t.Anchor); err != nil {
			return err
		}
		*out = append(*out, t)
	}
	return rows.Err()
}

// DeliveryStatusRow is one delivery_status lookup row — a status value plus the
// coverage policy it carries (read by a future check / coverage rollup).
type DeliveryStatusRow struct {
	Key                string
	Label              string
	Description        string
	Sequence           int
	CountsAsCovered    bool
	RequiresE2ETest    bool
	RequiresSharedTest bool
	RequiresMilestone  bool
}

const deliveryStatusCols = "`key`, COALESCE(label,''), COALESCE(description,''), sequence, " +
	"counts_as_covered, requires_e2e_test, requires_shared_test, requires_milestone"

func scanDeliveryStatus(s interface{ Scan(...any) error }, d *DeliveryStatusRow) error {
	return s.Scan(&d.Key, &d.Label, &d.Description, &d.Sequence,
		&d.CountsAsCovered, &d.RequiresE2ETest, &d.RequiresSharedTest, &d.RequiresMilestone)
}

// ListDeliveryStatuses returns the delivery_status lookup rows in sequence order.
func ListDeliveryStatuses(ctx context.Context, x Execer) ([]DeliveryStatusRow, error) {
	rows, err := x.QueryContext(ctx,
		"SELECT "+deliveryStatusCols+" FROM `plan_delivery_status` ORDER BY sequence, `key`")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DeliveryStatusRow
	for rows.Next() {
		var d DeliveryStatusRow
		if err := scanDeliveryStatus(rows, &d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeliveryStatusByKey returns one lookup row by its key (ok=false if absent).
func DeliveryStatusByKey(ctx context.Context, x Execer, key string) (DeliveryStatusRow, bool, error) {
	var d DeliveryStatusRow
	err := scanDeliveryStatus(
		x.QueryRowContext(ctx, "SELECT "+deliveryStatusCols+" FROM `plan_delivery_status` WHERE `key`=?", key), &d)
	if err == sql.ErrNoRows {
		return DeliveryStatusRow{}, false, nil
	}
	if err != nil {
		return DeliveryStatusRow{}, false, err
	}
	return d, true, nil
}

// EntityRefRow is one prose-derived cross-reference owned by a node.
type EntityRefRow struct {
	ID         string
	TargetType string
	TargetID   string
	Kind       string
}

// ListEntityRefsByOwner returns an owner's entity_ref rows, ordered by target.
func ListEntityRefsByOwner(ctx context.Context, x Execer, ownerType, ownerID string) ([]EntityRefRow, error) {
	rows, err := x.QueryContext(ctx,
		"SELECT id, target_type, target_id, kind FROM `req_entity_ref` WHERE owner_type=? AND owner_id=? ORDER BY target_type, target_id",
		ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EntityRefRow
	for rows.Next() {
		var r EntityRefRow
		if err := rows.Scan(&r.ID, &r.TargetType, &r.TargetID, &r.Kind); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
