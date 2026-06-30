package generate

import (
	"context"
	"sort"
	"strings"

	"github.com/endermalkoc/cusp/internal/refs"
	"github.com/endermalkoc/cusp/internal/store"
)

// Model is the format-agnostic view of the canonical graph that every renderer (Markdown,
// JSON, HTML, …) consumes. It is assembled once by Load, so a renderer never touches the
// store: assembly and formatting are separate concerns. The JSON tags double as the JSON
// serialization shape.
type Model struct {
	Domains      []*Domain      `json:"domains"`
	Specs        []*Spec        `json:"specs"`
	Entities     []*Entity      `json:"entities"`
	Terms        []*Term        `json:"glossary,omitempty"`
	Capabilities []*Capability  `json:"capabilities,omitempty"`
	Deliverables []*Deliverable `json:"deliverables,omitempty"`
	Views        []*View        `json:"views,omitempty"`
	Targets      []refs.Target  `json:"-"` // inline-ref resolution (doc renderers); not serialized
	Priorities   map[int]string `json:"-"` // level → label
	Nav          *Nav           `json:"-"` // full site-navigation tree (HTML chrome); not serialized
}

// Nav is the whole-project navigation tree — header data only (no bodies), so every HTML
// page can render the same always-visible sidebar. It mirrors the documents' actual on-disk
// path hierarchy (domain → sub-directories → docs), not just a flat domain→spec list, so a
// spec at `enrollment/student-detail/overview.md` nests under a "Student Detail" group. It is
// loaded in full even on the incremental fast path (a few cheap header queries), so a single
// re-rendered document still carries the complete navigation and stays byte-identical to a
// full rebuild.
type Nav struct {
	Root     []*NavNode        // top-level nodes in display order (domains, Entities, Glossary)
	DirLabel map[string]string // dir path → display name, for dirs that have an index page
}

// NavNode is one entry in the navigation tree. A node with an Href is a link (a document, or
// a directory that has an index page); a node with children but no Href is a bare directory
// grouping. Seg is the raw path segment, used to merge sibling docs into the same directory.
// Kind drives the type icon (domain | dir | spec | entity | entities | glossary).
type NavNode struct {
	Label    string
	Href     string // root-relative .html path; "" for a non-linking directory grouping
	Seg      string // raw path segment (for dir matching during tree construction)
	Kind     string // node type, for iconography
	Children []*NavNode
}

// HasIndex reports whether a directory path has its own index page (domains and the entities
// root do; arbitrary sub-directories do not).
func (n *Nav) HasIndex(dirPath string) bool { _, ok := n.DirLabel[dirPath]; return ok }

// SegLabel is the display label for a path segment: a domain/entities directory uses its
// proper name; any other segment is humanized from kebab/snake case.
func (n *Nav) SegLabel(dirPath, seg string) string {
	if l, ok := n.DirLabel[dirPath]; ok {
		return l
	}
	return humanizeSegment(seg)
}

// humanizeSegment turns a kebab/snake path segment into a Title-Cased label
// ("student-detail" → "Student Detail").
func humanizeSegment(seg string) string {
	words := strings.FieldsFunc(seg, func(r rune) bool { return r == '-' || r == '_' })
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	if len(words) == 0 {
		return seg
	}
	return strings.Join(words, " ")
}

// Domain is a top-level grouping of specs.
type Domain struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
}

// Spec is a feature specification with its prose sections, stories, and requirements.
// Path is the full document path (`<domain>/[dir/]<slug>.md`); renderers swap the
// extension as needed.
type Spec struct {
	Prefix       string         `json:"prefix,omitempty"`
	Slug         string         `json:"slug,omitempty"`
	Title        string         `json:"title,omitempty"`
	Domain       string         `json:"domain"`
	Status       string         `json:"status"`
	Created      string         `json:"created,omitempty"` // source created date (YYYY-MM-DD); "" when unknown
	Path         string         `json:"path"`
	Sections     []*Section     `json:"sections,omitempty"`
	Stories      []*Story       `json:"user_stories,omitempty"`
	Groups       []*Group       `json:"requirement_groups,omitempty"`
	Requirements []*Requirement `json:"requirements,omitempty"`
}

// Section is one curated prose section (overview, edge_cases, …).
type Section struct {
	Key      string `json:"key"`
	Title    string `json:"title,omitempty"`
	Level    int    `json:"level"`
	Position int    `json:"position"`
	Body     string `json:"body,omitempty"`
}

// Story is a user story plus its acceptance scenarios.
type Story struct {
	Position        int         `json:"position"`
	Title           string      `json:"title,omitempty"`
	Priority        int         `json:"priority"`
	AsA             string      `json:"as_a,omitempty"`
	IWant           string      `json:"i_want,omitempty"`
	SoThat          string      `json:"so_that,omitempty"`
	Narrative       string      `json:"narrative,omitempty"`
	WhyPriority     string      `json:"why_priority,omitempty"`
	IndependentTest string      `json:"independent_test,omitempty"`
	Scenarios       []*Scenario `json:"scenarios,omitempty"`
}

// Scenario is one Given/When/Then acceptance scenario.
type Scenario struct {
	Position int    `json:"position"`
	Given    string `json:"given,omitempty"`
	When     string `json:"when,omitempty"`
	Then     string `json:"then,omitempty"`
}

// Group is an FR group sub-header (title + interspersed note) over a spec's FR list.
type Group struct {
	ID       string `json:"-"`
	Position int    `json:"position"`
	Title    string `json:"title"`
	Notes    string `json:"notes,omitempty"`
}

// Requirement is one functional requirement. GroupID links it to a Group; MD renders only
// FRKey/Statement/GroupID, but the model carries the full row for richer data formats.
type Requirement struct {
	FRKey          string `json:"fr_key"`
	Number         int    `json:"number"`
	Suffix         string `json:"suffix,omitempty"`
	GroupID        string `json:"-"`
	Position       int    `json:"position,omitempty"`
	Statement      string `json:"statement,omitempty"`
	DeliveryStatus string `json:"delivery_status,omitempty"`
	Milestone      string `json:"milestone,omitempty"`
}

// Entity is a first-class shared-entity document with its prose sections.
type Entity struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Status      string     `json:"status"`
	DocPath     string     `json:"path"`
	Sections    []*Section `json:"sections,omitempty"`
}

// Term is a glossary term.
type Term struct {
	Slug       string   `json:"slug"`
	Term       string   `json:"term,omitempty"`
	Definition string   `json:"definition,omitempty"`
	DomainSlug string   `json:"domain,omitempty"`
	Aliases    []string `json:"aliases,omitempty"`
}

// Capability, Deliverable, and View are the planning layer (the "what to build" chain).
// Relationships are denormalized to display titles at Load time, so renderers stay pure and
// the JSON output is human-readable. NotionURL/BeadIDs carry the imported source pointers.
type Capability struct {
	ID           string   `json:"-"`
	Title        string   `json:"title"`
	Level        string   `json:"level,omitempty"` // domain|epic|capability
	DomainSlug   string   `json:"domain,omitempty"`
	ParentID     string   `json:"-"`
	ParentTitle  string   `json:"parent,omitempty"`
	Milestones   []string `json:"milestones,omitempty"`   // milestone slugs
	Deliverables []string `json:"deliverables,omitempty"` // deliverable titles
	NotionURL    string   `json:"notion_url,omitempty"`
}

// Deliverable is a unit of work (the external-task subject).
type Deliverable struct {
	ID           string   `json:"-"`
	Title        string   `json:"title"`
	Size         string   `json:"size,omitempty"`
	Status       string   `json:"status,omitempty"`
	AIReady      string   `json:"ai_ready,omitempty"`
	Milestone    string   `json:"milestone,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"` // titles
	Views        []string `json:"views,omitempty"`        // titles
	BlockedBy    []string `json:"blocked_by,omitempty"`   // titles
	NotionURL    string   `json:"notion_url,omitempty"`
	BeadIDs      string   `json:"bead_ids,omitempty"`
}

// View is a UI surface, optionally backed by a spec (the bridge into the spec corpus).
type View struct {
	ID           string   `json:"-"`
	Title        string   `json:"title"`
	Route        string   `json:"route,omitempty"`
	DomainSlug   string   `json:"domain,omitempty"`
	SpecPath     string   `json:"spec,omitempty"` // backing spec doc path (.md); "" when unlinked
	SpecTitle    string   `json:"spec_title,omitempty"`
	Deliverables []string `json:"deliverables,omitempty"` // titles
	NotionURL    string   `json:"notion_url,omitempty"`
}

// Load assembles the whole graph from the store into a Model. This is the single place
// that queries; every renderer reads the returned structs.
func Load(ctx context.Context, x store.Execer) (*Model, error) {
	m := &Model{}

	domains, err := store.ListDomains(ctx, x)
	if err != nil {
		return nil, err
	}
	for _, d := range domains {
		m.Domains = append(m.Domains, &Domain{Slug: d.Slug, Name: d.Name, Description: d.Description, Status: d.Status})
	}

	specRows, err := store.ListSpecs(ctx, x)
	if err != nil {
		return nil, err
	}
	for _, sr := range specRows {
		sp, err := loadSpecDoc(ctx, x, sr)
		if err != nil {
			return nil, err
		}
		m.Specs = append(m.Specs, sp)
	}

	entityRows, err := store.ListEntities(ctx, x)
	if err != nil {
		return nil, err
	}
	for _, er := range entityRows {
		e, err := loadEntityDoc(ctx, x, er)
		if err != nil {
			return nil, err
		}
		m.Entities = append(m.Entities, e)
	}

	termRows, err := store.ListGlossaryTerms(ctx, x)
	if err != nil {
		return nil, err
	}
	for _, t := range termRows {
		m.Terms = append(m.Terms, &Term{Slug: t.Slug, Term: t.Term, Definition: t.Definition, DomainSlug: t.DomainSlug, Aliases: t.Aliases})
	}

	if err := loadPlanning(ctx, x, m); err != nil {
		return nil, err
	}

	if err := loadShared(ctx, x, m); err != nil {
		return nil, err
	}
	return m, nil
}

// loadPlanning fills the planning layer (Capabilities/Deliverables/Views) with relationships
// resolved to display titles. It is only called by the full Load (not LoadDocs): the planning
// pages are whole-graph roll-ups, like the index pages, so they regenerate on a full rebuild.
func loadPlanning(ctx context.Context, x store.Execer, m *Model) error {
	capRows, err := store.ListCapabilities(ctx, x)
	if err != nil {
		return err
	}
	delivRows, err := store.ListDeliverables(ctx, x)
	if err != nil {
		return err
	}
	viewRows, err := store.ListPlanViews(ctx, x)
	if err != nil {
		return err
	}
	if len(capRows) == 0 && len(delivRows) == 0 && len(viewRows) == 0 {
		return nil
	}

	capTitle := map[string]string{}
	for _, c := range capRows {
		capTitle[c.ID] = c.Title
	}
	delivTitle := map[string]string{}
	for _, d := range delivRows {
		delivTitle[d.ID] = d.Title
	}
	viewTitle := map[string]string{}
	for _, v := range viewRows {
		viewTitle[v.ID] = v.Title
	}

	// External source pointers (Notion url, Bead ids), keyed by subject id.
	notionURL := map[string]string{}
	beadIDs := map[string]string{}
	extRefs, err := store.ListExternalRefsForSubjects(ctx, x)
	if err != nil {
		return err
	}
	for _, r := range extRefs {
		switch r.System {
		case "notion":
			notionURL[r.SubjectID] = r.URL
		case "beads":
			beadIDs[r.SubjectID] = r.ExternalID
		}
	}

	// Junctions → display-title lists per owner.
	capMs := map[string][]string{}
	if pairs, err := store.ListCapabilityMilestonePairs(ctx, x); err == nil {
		for _, p := range pairs {
			capMs[p.A] = append(capMs[p.A], p.B) // B is the milestone slug
		}
	} else {
		return err
	}
	capDl := map[string][]string{} // capability → deliverable titles
	dlCap := map[string][]string{} // deliverable → capability titles
	if pairs, err := store.ListCapabilityDeliverablePairs(ctx, x); err == nil {
		for _, p := range pairs {
			if t := delivTitle[p.B]; t != "" {
				capDl[p.A] = append(capDl[p.A], t)
			}
			if t := capTitle[p.A]; t != "" {
				dlCap[p.B] = append(dlCap[p.B], t)
			}
		}
	} else {
		return err
	}
	dlView := map[string][]string{} // deliverable → view titles
	viewDl := map[string][]string{} // view → deliverable titles
	if pairs, err := store.ListDeliverableViewPairs(ctx, x); err == nil {
		for _, p := range pairs {
			if t := viewTitle[p.B]; t != "" {
				dlView[p.A] = append(dlView[p.A], t)
			}
			if t := delivTitle[p.A]; t != "" {
				viewDl[p.B] = append(viewDl[p.B], t)
			}
		}
	} else {
		return err
	}
	dlBlocked := map[string][]string{} // deliverable → blocked-by titles
	if pairs, err := store.ListDeliverableDependencyPairs(ctx, x); err == nil {
		for _, p := range pairs {
			if t := delivTitle[p.B]; t != "" {
				dlBlocked[p.A] = append(dlBlocked[p.A], t)
			}
		}
	} else {
		return err
	}

	for _, c := range capRows {
		sort.Strings(capMs[c.ID])
		sort.Strings(capDl[c.ID])
		m.Capabilities = append(m.Capabilities, &Capability{
			ID: c.ID, Title: c.Title, Level: c.Level, DomainSlug: c.DomainSlug,
			ParentID: c.ParentID, ParentTitle: capTitle[c.ParentID],
			Milestones: capMs[c.ID], Deliverables: capDl[c.ID], NotionURL: notionURL[c.ID],
		})
	}
	for _, d := range delivRows {
		sort.Strings(dlCap[d.ID])
		sort.Strings(dlView[d.ID])
		sort.Strings(dlBlocked[d.ID])
		m.Deliverables = append(m.Deliverables, &Deliverable{
			ID: d.ID, Title: d.Title, Size: d.Size, Status: d.Status, AIReady: d.AIReady, Milestone: d.MilestoneSlug,
			Capabilities: dlCap[d.ID], Views: dlView[d.ID], BlockedBy: dlBlocked[d.ID],
			NotionURL: notionURL[d.ID], BeadIDs: beadIDs[d.ID],
		})
	}
	for _, v := range viewRows {
		specPath := ""
		if v.SpecSlug != "" {
			specPath = store.SpecDocPath(v.SpecDomain, v.SpecPath, v.SpecSlug)
		}
		sort.Strings(viewDl[v.ID])
		m.Views = append(m.Views, &View{
			ID: v.ID, Title: v.Title, Route: v.Route, DomainSlug: v.DomainSlug,
			SpecPath: specPath, SpecTitle: v.SpecTitle, Deliverables: viewDl[v.ID], NotionURL: notionURL[v.ID],
		})
	}
	return nil
}

// LoadDocs assembles a partial Model holding only the named specs and entities (plus the
// full shared Targets+Priorities, so inline cross-reference links still resolve). It is the
// fast path for incremental regeneration: render this model and keep only the spec/entity
// files — the index/glossary inputs are intentionally left empty, since a partial model
// cannot produce correct index pages (those are full-graph rollups).
func LoadDocs(ctx context.Context, x store.Execer, specIDs, entityIDs []string) (*Model, error) {
	m := &Model{}
	if len(specIDs) > 0 {
		want := make(map[string]bool, len(specIDs))
		for _, id := range specIDs {
			want[id] = true
		}
		specRows, err := store.ListSpecs(ctx, x)
		if err != nil {
			return nil, err
		}
		for _, sr := range specRows {
			if !want[sr.ID] {
				continue
			}
			sp, err := loadSpecDoc(ctx, x, sr)
			if err != nil {
				return nil, err
			}
			m.Specs = append(m.Specs, sp)
		}
	}
	if len(entityIDs) > 0 {
		want := make(map[string]bool, len(entityIDs))
		for _, id := range entityIDs {
			want[id] = true
		}
		entityRows, err := store.ListEntities(ctx, x)
		if err != nil {
			return nil, err
		}
		for _, er := range entityRows {
			if !want[er.ID] {
				continue
			}
			e, err := loadEntityDoc(ctx, x, er)
			if err != nil {
				return nil, err
			}
			m.Entities = append(m.Entities, e)
		}
	}
	if err := loadShared(ctx, x, m); err != nil {
		return nil, err
	}
	return m, nil
}

// loadSpecDoc reads one spec header row into a fully-populated *Spec (sections, stories +
// scenarios, requirement groups, requirements). Shared by the full Load and LoadDocs.
func loadSpecDoc(ctx context.Context, x store.Execer, sr store.SpecRow) (*Spec, error) {
	sp := &Spec{
		Prefix: sr.Prefix, Slug: sr.Slug, Title: sr.Title, Domain: sr.DomainSlug, Status: sr.Status,
		Created: sr.Created,
		Path:    store.SpecDocPath(sr.DomainSlug, sr.Path, sr.Slug),
	}
	secRows, err := store.ListSpecSections(ctx, x, sr.ID)
	if err != nil {
		return nil, err
	}
	sp.Sections = toSections(secRows)
	storyRows, err := store.ListStoriesBySpec(ctx, x, sr.ID)
	if err != nil {
		return nil, err
	}
	for _, st := range storyRows {
		s := &Story{
			Position: st.Position, Title: st.Title, Priority: st.Priority, AsA: st.AsA,
			IWant: st.IWant, SoThat: st.SoThat, Narrative: st.Narrative,
			WhyPriority: st.WhyPriority, IndependentTest: st.IndependentTest,
		}
		scnRows, err := store.ListScenariosByStory(ctx, x, st.ID)
		if err != nil {
			return nil, err
		}
		for _, sc := range scnRows {
			s.Scenarios = append(s.Scenarios, &Scenario{Position: sc.Position, Given: sc.Given, When: sc.When, Then: sc.Then})
		}
		sp.Stories = append(sp.Stories, s)
	}
	groupRows, err := store.ListReqGroups(ctx, x, sr.ID)
	if err != nil {
		return nil, err
	}
	for _, g := range groupRows {
		sp.Groups = append(sp.Groups, &Group{ID: g.ID, Position: g.Position, Title: g.Title, Notes: g.Notes})
	}
	reqRows, err := store.ListReqsBySpecID(ctx, x, sr.ID)
	if err != nil {
		return nil, err
	}
	for _, r := range reqRows {
		sp.Requirements = append(sp.Requirements, &Requirement{
			FRKey: r.FRKey, Number: r.Number, Suffix: r.Suffix, GroupID: r.GroupID,
			Position: r.Position, Statement: r.Statement, DeliveryStatus: r.DeliveryStatus, Milestone: r.Milestone,
		})
	}
	return sp, nil
}

// loadEntityDoc reads one entity header row into a fully-populated *Entity (its sections).
func loadEntityDoc(ctx context.Context, x store.Execer, er store.EntityRow) (*Entity, error) {
	e := &Entity{Name: er.Name, Description: er.Description, Status: er.Status, DocPath: er.DocPath}
	secRows, err := store.ListEntitySections(ctx, x, er.ID)
	if err != nil {
		return nil, err
	}
	e.Sections = toSections(secRows)
	return e, nil
}

// loadShared fills the model's cross-cutting inputs that every renderer needs regardless of
// which docs are present: the inline-ref target table, the priority labels, and the full
// navigation tree (for the HTML sidebar).
func loadShared(ctx context.Context, x store.Execer, m *Model) error {
	targets, err := store.ListRefTargets(ctx, x)
	if err != nil {
		return err
	}
	m.Targets = toTargets(targets)
	prio, err := loadPriorityLabels(ctx, x)
	if err != nil {
		return err
	}
	m.Priorities = prio
	nav, err := loadNav(ctx, x)
	if err != nil {
		return err
	}
	m.Nav = nav
	return nil
}

// loadNav assembles the full navigation tree from header rows only (no per-document children),
// so it is cheap enough to load on every mutation — including the fast path, which otherwise
// loads just the dirty documents. The tree has top-level sections — Specifications (all the
// domains and their specs) and Entities — with room for more later (Glossary appears when
// terms exist). Within Specifications a domain node (linked to its index page) holds its
// specs, nesting any sub-directories as grouping nodes, mirroring each document's real path.
func loadNav(ctx context.Context, x store.Execer) (*Nav, error) {
	nav := &Nav{DirLabel: map[string]string{}}

	// Section 1: Specifications — the existing root index is its landing page.
	specsRoot := &NavNode{Label: "Specifications", Href: "index.html", Kind: "specs"}
	nav.Root = append(nav.Root, specsRoot)

	domainRows, err := store.ListDomains(ctx, x)
	if err != nil {
		return nil, err
	}
	specRows, err := store.ListSpecs(ctx, x)
	if err != nil {
		return nil, err
	}
	// One node per domain (index-linked); specs nest under it by their path tail.
	domainNode := map[string]*NavNode{}
	for _, d := range domainRows {
		name := d.Name
		if name == "" {
			name = d.Slug
		}
		nav.DirLabel[d.Slug] = name
		n := &NavNode{Label: name, Href: d.Slug + "/index.html", Seg: d.Slug, Kind: "domain"}
		domainNode[d.Slug] = n
		specsRoot.Children = append(specsRoot.Children, n)
	}
	for _, s := range specRows {
		parent := domainNode[s.DomainSlug]
		if parent == nil {
			continue // a spec whose domain is missing has no place to hang
		}
		label := s.Title
		if label == "" {
			label = s.Slug
		}
		docPath := store.SpecDocPath(s.DomainSlug, s.Path, s.Slug) // <domain>/[dirs/]<slug>.md
		tail := strings.Split(strings.TrimSuffix(docPath, ".md"), "/")[1:]
		insertDoc(parent, s.DomainSlug, tail, label, "spec")
	}

	// Section 2: Planning — capabilities / deliverables / views roll-up pages (shown only when
	// planning data exists). Each sub-page appears only if it has rows.
	caps, delivs, views, err := store.PlanningCounts(ctx, x)
	if err != nil {
		return nil, err
	}
	if caps+delivs+views > 0 {
		planningNode := &NavNode{Label: "Planning", Href: "planning/index.html", Seg: "planning", Kind: "planning"}
		nav.DirLabel["planning"] = "Planning"
		if caps > 0 {
			planningNode.Children = append(planningNode.Children, &NavNode{Label: "Capabilities", Href: "planning/capabilities.html", Seg: "capabilities", Kind: "capability"})
		}
		if delivs > 0 {
			planningNode.Children = append(planningNode.Children, &NavNode{Label: "Deliverables", Href: "planning/deliverables.html", Seg: "deliverables", Kind: "deliverable"})
		}
		if views > 0 {
			planningNode.Children = append(planningNode.Children, &NavNode{Label: "Views", Href: "planning/views.html", Seg: "views", Kind: "view"})
		}
		nav.Root = append(nav.Root, planningNode)
	}

	// Section 3: Entities — under a single "entities/" tree, mirroring their doc paths.
	entitiesNode := &NavNode{Label: "Entities", Href: "entities/index.html", Seg: "entities", Kind: "entities"}
	nav.DirLabel["entities"] = "Entities"
	nav.Root = append(nav.Root, entitiesNode)
	entityRows, err := store.ListEntities(ctx, x)
	if err != nil {
		return nil, err
	}
	for _, e := range entityRows {
		tail := strings.Split(strings.TrimSuffix(e.DocPath, ".md"), "/")[1:] // drop "entities"
		insertDoc(entitiesNode, "entities", tail, e.Name, "entity")
	}

	termRows, err := store.ListGlossaryTerms(ctx, x)
	if err != nil {
		return nil, err
	}
	if len(termRows) > 0 {
		nav.Root = append(nav.Root, &NavNode{Label: "Glossary", Href: "glossary.html", Seg: "glossary", Kind: "glossary"})
	}
	return nav, nil
}

// insertDoc places a document under parent following its path tail (the segments below
// parent's directory). All but the last segment are directory groupings (created on demand
// and shared by siblings); the last is the document leaf, linked to its .html page. base is
// parent's directory path, so leaf/dir hrefs are built absolute from the site root. leafKind
// is the icon kind for the document leaf (spec | entity).
func insertDoc(parent *NavNode, base string, tail []string, leafLabel, leafKind string) {
	if len(tail) == 0 {
		return
	}
	if len(tail) == 1 {
		parent.Children = append(parent.Children, &NavNode{
			Label: leafLabel, Href: base + "/" + tail[0] + ".html", Seg: tail[0], Kind: leafKind,
		})
		return
	}
	seg := tail[0]
	dirPath := base + "/" + seg
	var dir *NavNode
	for _, c := range parent.Children {
		if c.Href == "" && c.Seg == seg {
			dir = c
			break
		}
	}
	if dir == nil {
		dir = &NavNode{Label: humanizeSegment(seg), Seg: seg, Kind: "dir"}
		parent.Children = append(parent.Children, dir)
	}
	insertDoc(dir, dirPath, tail[1:], leafLabel, leafKind)
}

func toSections(rows []store.SectionRow) []*Section {
	out := make([]*Section, len(rows))
	for i, r := range rows {
		out[i] = &Section{Key: r.Key, Title: r.Title, Level: r.Level, Position: r.Position, Body: r.Body}
	}
	return out
}

// toTargets adapts store ref-target rows to the refs resolver's Target shape.
func toTargets(rows []store.RefTargetRow) []refs.Target {
	out := make([]refs.Target, len(rows))
	for i, r := range rows {
		out[i] = refs.Target{Type: r.Type, Key: r.Key, ID: r.ID, DocPath: r.DocPath, Anchor: r.Anchor, Label: r.Label}
	}
	return out
}

// loadPriorityLabels loads the level→label map for the 0–4 priority taxonomy.
func loadPriorityLabels(ctx context.Context, x store.Execer) (map[int]string, error) {
	rows, err := store.ListPriorities(ctx, x)
	if err != nil {
		return nil, err
	}
	m := make(map[int]string, len(rows))
	for _, p := range rows {
		m[p.Level] = p.Label
	}
	return m, nil
}
