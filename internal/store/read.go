package store

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
)

// This file holds read queries used by the generator (internal/generate) to
// reconstruct documents from the canonical database. Plain reads, no transaction.

// ---- doc-path reconstruction ----------------------------------------------
// Neither req_spec nor ent_entity stores its full doc path: req_spec stores a
// directory (`path`) + filename stem (`slug`); ent_entity stores an optional
// sub-directory (`path`) and derives its filename from `name`.

var camelBoundary = regexp.MustCompile(`([a-z0-9])([A-Z])`)
var nonKebab = regexp.MustCompile(`[^a-z0-9]+`)

// kebabName is an entity name's doc filename stem: CamelCase boundaries and spaces
// become dashes, lowercased ("EventAttendance" → "event-attendance", "Group Tag" →
// "group-tag").
func kebabName(name string) string {
	s := camelBoundary.ReplaceAllString(name, "$1-$2")
	return strings.Trim(nonKebab.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// dirPrefix returns "" for an empty dir, else "dir/".
func dirPrefix(dir string) string {
	if dir == "" {
		return ""
	}
	return dir + "/"
}

// SpecDocPath reconstructs a spec's full docs path: <domain>/[path/]<slug>.md.
func SpecDocPath(domainSlug, path, slug string) string {
	return domainSlug + "/" + dirPrefix(path) + slug + ".md"
}

// EntityDocPath reconstructs an entity's full docs path: entities/[path/]<kebab-name>.md.
func EntityDocPath(path, name string) string {
	return "entities/" + dirPrefix(path) + kebabName(name) + ".md"
}

// SpecRow is a spec joined to its domain slug. Prose sections live in
// req_spec_section (typed by req_spec_section_type); the H1 is rendered from `title`.
type SpecRow struct {
	ID         string `json:"id"`
	DomainSlug string `json:"domain"`
	Prefix     string `json:"prefix,omitempty"`
	Path       string `json:"path"` // directory only (no filename); full = SpecDocPath(domain,path,slug)
	Slug       string `json:"slug"` // filename stem
	Title      string `json:"title,omitempty"`
	Status     string `json:"status"`
	Created    string `json:"created,omitempty"` // created_at as YYYY-MM-DD; "" when unknown (NULL)
}

// ListSpecs returns every spec with its domain slug, by path.
func ListSpecs(ctx context.Context, x Execer) ([]SpecRow, error) {
	rows, err := x.QueryContext(ctx, `
		SELECT s.id, d.slug, COALESCE(s.prefix,''), COALESCE(s.path,''), COALESCE(s.slug,''), COALESCE(s.title,''), s.status,
		       COALESCE(DATE_FORMAT(s.created_at,'%Y-%m-%d'),'')
		FROM `+"`req_spec`"+` s JOIN `+"`req_domain`"+` d ON s.domain_id = d.id
		ORDER BY s.path, s.slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpecRow
	for rows.Next() {
		var s SpecRow
		if err := rows.Scan(&s.ID, &s.DomainSlug, &s.Prefix, &s.Path, &s.Slug, &s.Title, &s.Status, &s.Created); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// StoryRow is a user story with the id needed to fetch its scenarios.
type StoryRow struct {
	ID              string
	Position        int
	Title           string
	Priority        int // 0–4 (req_priority)
	AsA             string
	IWant           string
	SoThat          string
	Narrative       string
	WhyPriority     string
	IndependentTest string
}

// ListStoriesBySpec returns a spec's user stories in document order.
func ListStoriesBySpec(ctx context.Context, x Execer, specID string) ([]StoryRow, error) {
	rows, err := x.QueryContext(ctx, `
		SELECT id, position, COALESCE(title,''), COALESCE(priority,0), COALESCE(as_a,''), COALESCE(i_want,''),
		       COALESCE(so_that,''), COALESCE(narrative,''), COALESCE(why_priority,''), COALESCE(independent_test,'')
		FROM `+"`req_user_story`"+` WHERE spec_id=? ORDER BY position`, specID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoryRow
	for rows.Next() {
		var s StoryRow
		if err := rows.Scan(&s.ID, &s.Position, &s.Title, &s.Priority, &s.AsA, &s.IWant, &s.SoThat, &s.Narrative, &s.WhyPriority, &s.IndependentTest); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// PriorityRow is one level of the standard 0–4 priority taxonomy (req_priority).
type PriorityRow struct {
	Level       int    `json:"level"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// ListPriorities returns the priority levels in order (0 = most urgent).
func ListPriorities(ctx context.Context, x Execer) ([]PriorityRow, error) {
	rows, err := x.QueryContext(ctx, "SELECT level, label, COALESCE(description,'') FROM `req_priority` ORDER BY level")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PriorityRow
	for rows.Next() {
		var p PriorityRow
		if err := rows.Scan(&p.Level, &p.Label, &p.Description); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PriorityByLevel returns one priority level (ok=false if absent) — the validation point.
func PriorityByLevel(ctx context.Context, x Execer, level int) (PriorityRow, bool, error) {
	var p PriorityRow
	err := x.QueryRowContext(ctx,
		"SELECT level, label, COALESCE(description,'') FROM `req_priority` WHERE level=?", level).
		Scan(&p.Level, &p.Label, &p.Description)
	if err == sql.ErrNoRows {
		return PriorityRow{}, false, nil
	}
	if err != nil {
		return PriorityRow{}, false, err
	}
	return p, true, nil
}

// ScenarioRow is one acceptance scenario.
type ScenarioRow struct {
	Position int
	Given    string
	When     string
	Then     string
}

// ListScenariosByStory returns a story's acceptance scenarios in order.
func ListScenariosByStory(ctx context.Context, x Execer, storyID string) ([]ScenarioRow, error) {
	rows, err := x.QueryContext(ctx, `
		SELECT position, COALESCE(`+"`given`"+`,''), COALESCE(`+"`when`"+`,''), COALESCE(`+"`then`"+`,'')
		FROM `+"`req_acceptance_scenario`"+` WHERE user_story_id=? ORDER BY position`, storyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScenarioRow
	for rows.Next() {
		var s ScenarioRow
		if err := rows.Scan(&s.Position, &s.Given, &s.When, &s.Then); err != nil {
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
// slug resolved (empty when none).
func ListReqsBySpecID(ctx context.Context, x Execer, specID string) ([]ReqRow, error) {
	rows, err := x.QueryContext(ctx, `
		SELECT r.fr_key, r.number, COALESCE(r.suffix,''), COALESCE(r.group_id,''), COALESCE(r.position,0), COALESCE(r.statement,''),
		       COALESCE(r.delivery_status,''), COALESCE(m.slug,'')
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

// ReqGroupRow is an FR group title + notes.
type ReqGroupRow struct {
	ID       string
	Position int
	Title    string
	Notes    string
}

// ListReqGroups returns a spec's FR groups ordered by position.
func ListReqGroups(ctx context.Context, x Execer, specID string) ([]ReqGroupRow, error) {
	rows, err := x.QueryContext(ctx, `
		SELECT id, position, title, COALESCE(notes,'') FROM `+"`req_requirement_group`"+`
		WHERE spec_id=? ORDER BY position`, specID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReqGroupRow
	for rows.Next() {
		var g ReqGroupRow
		if err := rows.Scan(&g.ID, &g.Position, &g.Title, &g.Notes); err != nil {
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

// ListEntities returns entities ordered by name, with their full doc path
// reconstructed (entities/[path/]<kebab-name>.md).
func ListEntities(ctx context.Context, x Execer) ([]EntityRow, error) {
	rows, err := x.QueryContext(ctx, `
		SELECT e.id, e.name, COALESCE(e.description,''), e.status, COALESCE(e.path,'')
		FROM `+"`ent_entity`"+` e
		ORDER BY e.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EntityRow
	for rows.Next() {
		var e EntityRow
		var subdir string
		if err := rows.Scan(&e.ID, &e.Name, &e.Description, &e.Status, &subdir); err != nil {
			return nil, err
		}
		e.DocPath = EntityDocPath(subdir, e.Name)
		out = append(out, e)
	}
	return out, rows.Err()
}

// SectionTypeRow is one curated section-type lookup row: the title/level/position
// that drive how a section of this type renders. Shared shape across the spec and entity
// vocabularies (req_spec_section_type / ent_entity_section_type).
type SectionTypeRow struct {
	Key         string
	Title       string
	Level       int
	Position    int
	Description string
	Origin      string
}

// SectionTypeRow.Key maps to the `slug` column (named slug, not the reserved word key).
const sectionTypeCols = "slug, COALESCE(title,''), level, position, COALESCE(description,''), origin"

func scanSectionType(s interface{ Scan(...any) error }, t *SectionTypeRow) error {
	return s.Scan(&t.Key, &t.Title, &t.Level, &t.Position, &t.Description, &t.Origin)
}

func listSectionTypes(ctx context.Context, x Execer, typeTable string) ([]SectionTypeRow, error) {
	rows, err := x.QueryContext(ctx,
		"SELECT "+sectionTypeCols+" FROM `"+typeTable+"` ORDER BY position, slug")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SectionTypeRow
	for rows.Next() {
		var t SectionTypeRow
		if err := scanSectionType(rows, &t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func sectionTypeByKey(ctx context.Context, x Execer, typeTable, key string) (SectionTypeRow, bool, error) {
	var t SectionTypeRow
	err := scanSectionType(
		x.QueryRowContext(ctx, "SELECT "+sectionTypeCols+" FROM `"+typeTable+"` WHERE slug=?", key), &t)
	if err == sql.ErrNoRows {
		return SectionTypeRow{}, false, nil
	}
	if err != nil {
		return SectionTypeRow{}, false, err
	}
	return t, true, nil
}

// ListSpecSectionTypes / ListEntitySectionTypes return the curated vocabulary in render order.
func ListSpecSectionTypes(ctx context.Context, x Execer) ([]SectionTypeRow, error) {
	return listSectionTypes(ctx, x, "req_spec_section_type")
}
func ListEntitySectionTypes(ctx context.Context, x Execer) ([]SectionTypeRow, error) {
	return listSectionTypes(ctx, x, "ent_entity_section_type")
}

// SpecSectionTypeByKey / EntitySectionTypeByKey resolve one type (ok=false if absent) —
// the friction point: an interactive write rejects a section whose type does not exist.
func SpecSectionTypeByKey(ctx context.Context, x Execer, key string) (SectionTypeRow, bool, error) {
	return sectionTypeByKey(ctx, x, "req_spec_section_type", key)
}
func EntitySectionTypeByKey(ctx context.Context, x Execer, key string) (SectionTypeRow, bool, error) {
	return sectionTypeByKey(ctx, x, "ent_entity_section_type", key)
}

// SectionRow is one rendered prose section: the body plus the title/level/position
// looked up from its curated type (joined). Returned in canonical render order.
type SectionRow struct {
	Key      string
	Title    string
	Level    int
	Position int
	Body     string
}

func listSections(ctx context.Context, x Execer, sectionTable, typeTable, ownerCol, ownerID string) ([]SectionRow, error) {
	rows, err := x.QueryContext(ctx,
		"SELECT t.slug, COALESCE(t.title,''), t.level, t.position, COALESCE(s.body,'') "+
			"FROM `"+sectionTable+"` s JOIN `"+typeTable+"` t ON t.slug = s.section_type_slug "+
			"WHERE s."+ownerCol+"=? ORDER BY t.position, t.slug", ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SectionRow
	for rows.Next() {
		var r SectionRow
		if err := rows.Scan(&r.Key, &r.Title, &r.Level, &r.Position, &r.Body); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListSpecSections / ListEntitySections return an owner's prose sections joined to their
// type, in canonical render order (SectionType.position).
func ListSpecSections(ctx context.Context, x Execer, specID string) ([]SectionRow, error) {
	return listSections(ctx, x, "req_spec_section", "req_spec_section_type", "spec_id", specID)
}
func ListEntitySections(ctx context.Context, x Execer, entityID string) ([]SectionRow, error) {
	return listSections(ctx, x, "ent_entity_section", "ent_entity_section_type", "entity_id", entityID)
}

// SpecIDByPrefix resolves a spec's id by its FR prefix (ok=false if absent).
func SpecIDByPrefix(ctx context.Context, x Execer, prefix string) (string, bool, error) {
	var id string
	err := x.QueryRowContext(ctx, "SELECT id FROM `req_spec` WHERE prefix=?", prefix).Scan(&id)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return id, err == nil, err
}

// EntityIDByName resolves an entity's id by its (unique) name (ok=false if absent).
func EntityIDByName(ctx context.Context, x Execer, name string) (string, bool, error) {
	var id string
	err := x.QueryRowContext(ctx, "SELECT id FROM `ent_entity` WHERE name=?", name).Scan(&id)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return id, err == nil, err
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
	Key     string // the [[TYPE:key]] key: slug | prefix-or-path | fr_key | name
	ID      string
	DocPath string // generated page path; requirement = owning spec's path; "" when none
	Anchor  string // in-page anchor (requirement = fr-key slug); "" otherwise
	Label   string // human display title (spec title / entity or domain name / fr_key); "" if none
}

// ListRefTargets returns every resolvable cross-reference target. Specs appear twice
// — keyed by prefix (when set) and by path — so `[[SPEC:ATT]]` and
// `[[SPEC:scheduling/events/take-attendance.md]]` both resolve.
func ListRefTargets(ctx context.Context, x Execer) ([]RefTargetRow, error) {
	var out []RefTargetRow
	// fullPath reconstructs a spec's full docs path: <domain>/[path/]<slug>.md (path is
	// the directory only; the filename is slug.md — migration 0017).
	const fullPath = "CONCAT(d.slug,'/',IF(COALESCE(s.path,'')='','',CONCAT(s.path,'/')),COALESCE(s.slug,''),'.md')"
	// Each query selects (type, key, id, doc_path, anchor, label) — label is the human title
	// used to render a clean link text (so a source link whose text was a path shows the
	// title, not `foo.md`).
	queries := []string{
		"SELECT 'domain', slug, id, '', '', COALESCE(name,slug) FROM `req_domain`",
		"SELECT 'spec', s.prefix, s.id, " + fullPath + ", '', COALESCE(s.title,s.prefix) FROM `req_spec` s JOIN `req_domain` d ON s.domain_id=d.id WHERE s.prefix IS NOT NULL AND s.prefix<>''",
		"SELECT 'spec', " + fullPath + ", s.id, " + fullPath + ", '', COALESCE(s.title,s.slug) FROM `req_spec` s JOIN `req_domain` d ON s.domain_id=d.id",
		"SELECT 'requirement', r.fr_key, r.id, " + fullPath + ", LOWER(r.fr_key), r.fr_key " +
			"FROM `req_requirement` r JOIN `req_spec` s ON r.spec_id = s.id JOIN `req_domain` d ON s.domain_id=d.id WHERE r.fr_key IS NOT NULL AND r.fr_key<>''",
		"SELECT 'milestone', slug, id, '', '', COALESCE(name,slug) FROM `plan_milestone`",
		// Glossary terms resolve by slug and by alias; both link to glossary.md#slug.
		"SELECT 'glossary_term', slug, id, 'glossary.md', slug, COALESCE(term,slug) FROM `req_glossary_term`",
		"SELECT 'glossary_term', a.alias, t.id, 'glossary.md', t.slug, COALESCE(t.term,t.slug) " +
			"FROM `req_glossary_alias` a JOIN `req_glossary_term` t ON a.term_id = t.id",
	}
	for _, q := range queries {
		if err := scanRefTargets(ctx, x, &out, q); err != nil {
			return nil, err
		}
	}
	// Entity targets: the doc filename is derived from the name (kebab), which SQL can't
	// build, so reconstruct the path in Go.
	rows, err := x.QueryContext(ctx, "SELECT name, id, COALESCE(path,'') FROM `ent_entity`")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name, id, subdir string
		if err := rows.Scan(&name, &id, &subdir); err != nil {
			return nil, err
		}
		out = append(out, RefTargetRow{Type: "entity", Key: name, ID: id, DocPath: EntityDocPath(subdir, name), Label: name})
	}
	return out, rows.Err()
}

func scanRefTargets(ctx context.Context, x Execer, out *[]RefTargetRow, query string) error {
	rows, err := x.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var t RefTargetRow
		if err := rows.Scan(&t.Type, &t.Key, &t.ID, &t.DocPath, &t.Anchor, &t.Label); err != nil {
			return err
		}
		*out = append(*out, t)
	}
	return rows.Err()
}

// EdgeEndpoint is one edge's resolved polymorphic endpoints (no kind/id) — enough to walk
// the graph for cycle detection.
type EdgeEndpoint struct{ FromType, FromID, ToType, ToID string }

// ListEdgesOfKind returns every edge of one kind on the current branch, for the acyclicity
// check before adding a new edge of that kind.
func ListEdgesOfKind(ctx context.Context, x Execer, kind string) ([]EdgeEndpoint, error) {
	rows, err := x.QueryContext(ctx,
		"SELECT from_type, from_id, to_type, to_id FROM `req_edge` WHERE kind=?", kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EdgeEndpoint
	for rows.Next() {
		var e EdgeEndpoint
		if err := rows.Scan(&e.FromType, &e.FromID, &e.ToType, &e.ToID); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// EdgeRow is one edge with its kind — for impact / graph traversal.
type EdgeRow struct{ FromType, FromID, ToType, ToID, Kind string }

// ListAllEdges returns every edge on the current branch (all kinds).
func ListAllEdges(ctx context.Context, x Execer) ([]EdgeRow, error) {
	rows, err := x.QueryContext(ctx, "SELECT from_type, from_id, to_type, to_id, kind FROM `req_edge`")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EdgeRow
	for rows.Next() {
		var e EdgeRow
		if err := rows.Scan(&e.FromType, &e.FromID, &e.ToType, &e.ToID, &e.Kind); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// EntityRefPair is one prose-derived reference as both endpoints (owner → target), by ids
// — for impact traversal (which needs both directions).
type EntityRefPair struct{ OwnerType, OwnerID, TargetType, TargetID string }

// ListEntityRefsFor returns every entity_ref with the given node as either owner or
// target — both directions of the prose-derived references touching it.
func ListEntityRefsFor(ctx context.Context, x Execer, nodeType, nodeID string) ([]EntityRefPair, error) {
	rows, err := x.QueryContext(ctx,
		"SELECT owner_type, owner_id, target_type, target_id FROM `req_entity_ref` "+
			"WHERE (owner_type=? AND owner_id=?) OR (target_type=? AND target_id=?)",
		nodeType, nodeID, nodeType, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EntityRefPair
	for rows.Next() {
		var r EntityRefPair
		if err := rows.Scan(&r.OwnerType, &r.OwnerID, &r.TargetType, &r.TargetID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
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

// DeliveryStatusRow.Key maps to the `slug` column (named slug, not the reserved word key; migration 0014).
const deliveryStatusCols = "slug, COALESCE(label,''), COALESCE(description,''), sequence, " +
	"counts_as_covered, requires_e2e_test, requires_shared_test, requires_milestone"

func scanDeliveryStatus(s interface{ Scan(...any) error }, d *DeliveryStatusRow) error {
	return s.Scan(&d.Key, &d.Label, &d.Description, &d.Sequence,
		&d.CountsAsCovered, &d.RequiresE2ETest, &d.RequiresSharedTest, &d.RequiresMilestone)
}

// ListDeliveryStatuses returns the delivery_status lookup rows in sequence order.
func ListDeliveryStatuses(ctx context.Context, x Execer) ([]DeliveryStatusRow, error) {
	rows, err := x.QueryContext(ctx,
		"SELECT "+deliveryStatusCols+" FROM `plan_delivery_status` ORDER BY sequence, slug")
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
		x.QueryRowContext(ctx, "SELECT "+deliveryStatusCols+" FROM `plan_delivery_status` WHERE slug=?", key), &d)
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
}

// ListEntityRefsByOwner returns an owner's entity_ref rows, ordered by target.
func ListEntityRefsByOwner(ctx context.Context, x Execer, ownerType, ownerID string) ([]EntityRefRow, error) {
	rows, err := x.QueryContext(ctx,
		"SELECT id, target_type, target_id FROM `req_entity_ref` WHERE owner_type=? AND owner_id=? ORDER BY target_type, target_id",
		ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EntityRefRow
	for rows.Next() {
		var r EntityRefRow
		if err := rows.Scan(&r.ID, &r.TargetType, &r.TargetID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
