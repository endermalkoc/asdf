// Package notion is the source adapter for a Notion planning workspace: the
// Capabilities / Deliverables / Views databases that track *what to build*. It maps
// them onto Cusp's planning layer (Capability → Deliverable → View, plus Domain and
// Milestone) and produces an importer.Graph + a Report.
//
// Two sources, same mapping:
//   - Parse(ctx, Config) queries the Notion REST API (a token + the three database ids);
//   - ParseDir(dir) reads saved query responses (capabilities.json / deliverables.json /
//     views.json) — a no-network path used by tests and offline imports.
//
// The mapping is deterministic and read-only: it never mints ULIDs (the apply pass
// derives deterministic ids from each page's stable Notion id) and never writes.
package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/endermalkoc/cusp/internal/importer"
)

// Default Notion database ids for the tutor workspace (see
// tutor/.claude/references/notion-api.md). Overridable via Config.
const (
	DefaultCapabilitiesDB = "f1dc62bd-3906-4d4b-a4e3-4ab21f1b523c"
	DefaultDeliverablesDB = "1fb2da82-755c-4adb-a07f-1ee09093aa66"
	DefaultViewsDB        = "d84e83c4-4b62-4de5-ab20-a053cd96a62b"
	defaultAPIVersion     = "2022-06-28"
)

// Notion property names (the workspace's column labels). Hardcoded to this workspace's
// schema, as the tutor adapter hardcodes its corpus conventions; a future mapping spec
// can make these configurable.
const (
	propLevel        = "Level"
	propDomain       = "Domain"
	propMilestone    = "Milestone"
	propParentItem   = "Parent item"
	propDeliverables = "Deliverables"
	propCapabilities = "Capabilities"
	propSize         = "Size"
	propStatus       = "Status"
	propAIReady      = "AI-Ready"
	propView         = "View"
	propBlockedBy    = "Blocked by"
	propBeadIDs      = "Bead IDs"
	propRoute        = "Route"
	propSpecFile     = "Spec File"
)

// Config selects the source databases and how to reach the API.
type Config struct {
	Token          string // Notion integration token (required for Parse)
	Version        string // Notion-Version header; default defaultAPIVersion
	CapabilitiesDB string // default DefaultCapabilitiesDB
	DeliverablesDB string // default DefaultDeliverablesDB
	ViewsDB        string // default DefaultViewsDB
	HTTPClient     *http.Client
}

func (c *Config) applyDefaults() {
	if c.Version == "" {
		c.Version = defaultAPIVersion
	}
	if c.CapabilitiesDB == "" {
		c.CapabilitiesDB = DefaultCapabilitiesDB
	}
	if c.DeliverablesDB == "" {
		c.DeliverablesDB = DefaultDeliverablesDB
	}
	if c.ViewsDB == "" {
		c.ViewsDB = DefaultViewsDB
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
}

// Page is the decoded subset of a Notion page we map from.
type Page struct {
	ID         string              `json:"id"`
	URL        string              `json:"url"`
	Properties map[string]Property `json:"properties"`
}

// Property is the decoded subset of a Notion property value (only the value kinds the
// planning databases use).
type Property struct {
	Type        string         `json:"type"`
	Title       []richText     `json:"title"`
	RichText    []richText     `json:"rich_text"`
	Select      *selectOption  `json:"select"`
	MultiSelect []selectOption `json:"multi_select"`
	Relation    []relationRef  `json:"relation"`
}

type richText struct {
	PlainText string `json:"plain_text"`
}
type selectOption struct {
	Name string `json:"name"`
}
type relationRef struct {
	ID string `json:"id"`
}

// titleAny returns the page's title, found by property type (robust to the title
// column's name differing per database).
func (p Page) titleAny() string {
	for _, prop := range p.Properties {
		if prop.Type == "title" {
			return joinRich(prop.Title)
		}
	}
	return ""
}

func (p Page) text(name string) string { return joinRich(p.Properties[name].RichText) }

func (p Page) sel(name string) string {
	if s := p.Properties[name].Select; s != nil {
		return s.Name
	}
	return ""
}

func (p Page) multi(name string) []string {
	var out []string
	for _, o := range p.Properties[name].MultiSelect {
		if o.Name != "" {
			out = append(out, o.Name)
		}
	}
	return out
}

func (p Page) rel(name string) []string {
	var out []string
	for _, r := range p.Properties[name].Relation {
		if r.ID != "" {
			out = append(out, r.ID)
		}
	}
	return out
}

func joinRich(rt []richText) string {
	var b strings.Builder
	for _, r := range rt {
		b.WriteString(r.PlainText)
	}
	return strings.TrimSpace(b.String())
}

// BuildGraph maps the three page sets onto the Cusp planning layer. It is pure: no
// network, no database, deterministic ordering.
func BuildGraph(caps, delivs, views []Page) (*importer.Graph, *importer.Report) {
	g := &importer.Graph{}
	rep := &importer.Report{Counts: map[string]int{}, Coverage: map[string]int{}}

	domains := map[string]string{} // slug → label
	milestones := map[string]bool{}
	usedFallbackDomain := false

	domainSlug := func(label string) string {
		if strings.TrimSpace(label) == "" {
			usedFallbackDomain = true
			return "unassigned"
		}
		s := slugify(label)
		if s == "" {
			usedFallbackDomain = true
			return "unassigned"
		}
		domains[s] = label
		return s
	}
	addMilestone := func(m string) {
		if strings.TrimSpace(m) != "" {
			milestones[m] = true
		}
	}

	for _, p := range caps {
		c := importer.Capability{
			SourceID:             p.ID,
			SourceURL:            p.URL,
			Title:                p.titleAny(),
			Level:                p.sel(propLevel),
			DomainSlug:           domainSlug(p.sel(propDomain)),
			ParentSourceID:       first(p.rel(propParentItem)),
			MilestoneSlugs:       p.multi(propMilestone),
			DeliverableSourceIDs: p.rel(propDeliverables),
		}
		for _, m := range c.MilestoneSlugs {
			addMilestone(m)
		}
		g.Capabilities = append(g.Capabilities, c)
	}

	for _, p := range delivs {
		d := importer.Deliverable{
			SourceID:            p.ID,
			SourceURL:           p.URL,
			Title:               p.titleAny(),
			Size:                normSize(p.sel(propSize)),
			Status:              normStatus(p.sel(propStatus)),
			AIReady:             normAIReady(p.sel(propAIReady)),
			MilestoneSlug:       p.sel(propMilestone),
			CapabilitySourceIDs: p.rel(propCapabilities),
			ViewSourceIDs:       p.rel(propView),
			BlockedBySourceIDs:  p.rel(propBlockedBy),
			BeadIDs:             p.text(propBeadIDs),
		}
		addMilestone(d.MilestoneSlug)
		g.Deliverables = append(g.Deliverables, d)
	}

	for _, p := range views {
		v := importer.View{
			SourceID:             p.ID,
			SourceURL:            p.URL,
			Title:                p.titleAny(),
			Route:                p.text(propRoute),
			DomainSlug:           domainSlug(p.sel(propDomain)),
			SpecFile:             p.text(propSpecFile),
			DeliverableSourceIDs: p.rel(propDeliverables),
		}
		g.Views = append(g.Views, v)
	}

	if usedFallbackDomain {
		domains["unassigned"] = "Unassigned"
		rep.Add(importer.SevWarn, "domain-missing",
			"one or more capabilities/views had no Domain — assigned to the 'unassigned' domain", "")
	}

	// Domains + milestones (sorted for deterministic output).
	for slug, label := range domains {
		g.Domains = append(g.Domains, importer.Domain{Slug: slug, Name: label})
	}
	sort.Slice(g.Domains, func(i, j int) bool { return g.Domains[i].Slug < g.Domains[j].Slug })
	for m := range milestones {
		g.Milestones = append(g.Milestones, importer.Milestone{Slug: m})
	}
	sort.Slice(g.Milestones, func(i, j int) bool { return g.Milestones[i].Slug < g.Milestones[j].Slug })

	rep.Counts["domains"] = len(g.Domains)
	rep.Counts["milestones"] = len(g.Milestones)
	rep.Counts["capabilities"] = len(g.Capabilities)
	rep.Counts["deliverables"] = len(g.Deliverables)
	rep.Counts["views"] = len(g.Views)

	buildFindings(g, rep)
	return g, rep
}

// buildFindings cross-checks the graph for danglers (relations whose target page is
// absent from the imported set) and unresolved soft references.
func buildFindings(g *importer.Graph, rep *importer.Report) {
	capSet := map[string]bool{}
	for _, c := range g.Capabilities {
		capSet[c.SourceID] = true
	}
	delivSet := map[string]bool{}
	for _, d := range g.Deliverables {
		delivSet[d.SourceID] = true
	}
	for _, c := range g.Capabilities {
		if c.ParentSourceID != "" && !capSet[c.ParentSourceID] {
			rep.Add(importer.SevWarn, "dangling-parent",
				"capability parent not found in the imported set (left unset)", c.Title)
		}
	}
	for _, d := range g.Deliverables {
		for _, bs := range d.BlockedBySourceIDs {
			if !delivSet[bs] {
				rep.Add(importer.SevWarn, "dangling-dependency",
					"'blocked by' deliverable not in the imported set (dropped)", d.Title)
			}
		}
	}
	specRefs := 0
	for _, v := range g.Views {
		if v.SpecFile != "" {
			specRefs++
		}
	}
	if specRefs > 0 {
		rep.Add(importer.SevInfo, "view-spec-link",
			fmt.Sprintf("%d views cite a Spec File — linked to a spec only when exactly one spec slug matches", specRefs), "")
	}
}

func first(s []string) string {
	if len(s) > 0 {
		return s[0]
	}
	return ""
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// slugify lowercases a label and collapses any run of non-alphanumerics to a single
// hyphen ("Website & Public Presence" → "website-public-presence").
func slugify(s string) string {
	return strings.Trim(nonAlnum.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// normSize keeps S|M|L|XL (uppercased), dropping blank/placeholder values.
func normSize(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch s {
	case "S", "M", "L", "XL":
		return s
	}
	return ""
}

// normStatus maps a Notion status to the deliverable enum; a blank or em-dash
// placeholder ("—") becomes the default "proposed".
func normStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "—", "-", "–":
		return "proposed"
	case "proposed":
		return "proposed"
	case "specced":
		return "specced"
	case "wired":
		return "wired"
	case "built":
		return "built"
	case "ship":
		return "ship"
	}
	return strings.ToLower(strings.TrimSpace(s))
}

// normAIReady maps Yes/No/N-A to the ai_ready enum (yes|no|na); blank stays blank.
func normAIReady(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "yes":
		return "yes"
	case "no":
		return "no"
	case "n/a", "na", "n.a.":
		return "na"
	}
	return ""
}

// ---- API source ------------------------------------------------------------

// Parse queries the three Notion databases and builds the planning graph.
func Parse(ctx context.Context, cfg Config) (*importer.Graph, *importer.Report, error) {
	cfg.applyDefaults()
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, nil, fmt.Errorf("a Notion token is required (set --token or NOTION_API_KEY)")
	}
	caps, err := cfg.fetchAll(ctx, cfg.CapabilitiesDB)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch capabilities: %w", err)
	}
	delivs, err := cfg.fetchAll(ctx, cfg.DeliverablesDB)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch deliverables: %w", err)
	}
	views, err := cfg.fetchAll(ctx, cfg.ViewsDB)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch views: %w", err)
	}
	g, rep := BuildGraph(caps, delivs, views)
	return g, rep, nil
}

// fetchAll pages through a database query, returning every page.
func (c *Config) fetchAll(ctx context.Context, dbID string) ([]Page, error) {
	var out []Page
	cursor := ""
	for {
		body := map[string]any{"page_size": 100}
		if cursor != "" {
			body["start_cursor"] = cursor
		}
		raw, _ := json.Marshal(body)
		url := "https://api.notion.com/v1/databases/" + dbID + "/query"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Notion-Version", c.Version)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		var page struct {
			Results    []Page `json:"results"`
			HasMore    bool   `json:"has_more"`
			NextCursor string `json:"next_cursor"`
			Message    string `json:"message"`
		}
		dec := json.NewDecoder(resp.Body)
		decErr := dec.Decode(&page)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			if page.Message != "" {
				return nil, fmt.Errorf("notion API %s: %s", resp.Status, page.Message)
			}
			return nil, fmt.Errorf("notion API %s", resp.Status)
		}
		if decErr != nil {
			return nil, decErr
		}
		out = append(out, page.Results...)
		if !page.HasMore || page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	return out, nil
}

// ---- directory source (offline) --------------------------------------------

// ParseDir builds the planning graph from saved Notion query responses in dir:
// capabilities.json, deliverables.json, views.json. Each file may be a raw query
// response ({"results":[…]}) or a bare array of pages.
func ParseDir(dir string) (*importer.Graph, *importer.Report, error) {
	caps, err := readPages(filepath.Join(dir, "capabilities.json"))
	if err != nil {
		return nil, nil, err
	}
	delivs, err := readPages(filepath.Join(dir, "deliverables.json"))
	if err != nil {
		return nil, nil, err
	}
	views, err := readPages(filepath.Join(dir, "views.json"))
	if err != nil {
		return nil, nil, err
	}
	g, rep := BuildGraph(caps, delivs, views)
	return g, rep, nil
}

func readPages(path string) ([]Page, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Try the query-response wrapper first, then a bare array.
	var wrapped struct {
		Results []Page `json:"results"`
	}
	if err := json.Unmarshal(b, &wrapped); err == nil && wrapped.Results != nil {
		return wrapped.Results, nil
	}
	var bare []Page
	if err := json.Unmarshal(b, &bare); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return bare, nil
}
