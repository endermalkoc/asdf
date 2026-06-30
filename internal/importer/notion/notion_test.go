package notion

import (
	"encoding/json"
	"testing"

	"github.com/endermalkoc/cusp/internal/importer"
)

func hasFinding(rep *importer.Report, category string) bool {
	for _, f := range rep.Findings {
		if f.Category == category {
			return true
		}
	}
	return false
}

// sampleResp is a minimal Notion query response with the property shapes the planning
// databases use, exercised through the JSON decoder (so the struct tags are tested too).
func decode(t *testing.T, body string) []Page {
	t.Helper()
	var resp struct {
		Results []Page `json:"results"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.Results
}

func TestBuildGraph(t *testing.T) {
	caps := decode(t, `{"results":[
      {"id":"cap-1","url":"https://notion/cap-1","properties":{
        "Capability":{"type":"title","title":[{"plain_text":"Lessons"}]},
        "Level":{"type":"select","select":{"name":"capability"}},
        "Domain":{"type":"select","select":{"name":"Learning"}},
        "Milestone":{"type":"multi_select","multi_select":[{"name":"M1"},{"name":"M2"}]},
        "Parent item":{"type":"relation","relation":[{"id":"cap-0"}]},
        "Deliverables":{"type":"relation","relation":[{"id":"del-1"}]}
      }},
      {"id":"cap-0","url":"https://notion/cap-0","properties":{
        "Capability":{"type":"title","title":[{"plain_text":"Learning"}]},
        "Level":{"type":"select","select":{"name":"domain"}},
        "Domain":{"type":"select","select":{"name":"Learning"}}
      }}
    ]}`)

	delivs := decode(t, `{"results":[
      {"id":"del-1","url":"https://notion/del-1","properties":{
        "Deliverable":{"type":"title","title":[{"plain_text":"Attendance reminder"}]},
        "Size":{"type":"select","select":{"name":"S"}},
        "Status":{"type":"select","select":{"name":"—"}},
        "AI-Ready":{"type":"select","select":{"name":"Yes"}},
        "Milestone":{"type":"select","select":{"name":"M3"}},
        "Capabilities":{"type":"relation","relation":[{"id":"cap-1"}]},
        "View":{"type":"relation","relation":[{"id":"view-1"}]},
        "Blocked by":{"type":"relation","relation":[{"id":"del-2"}]},
        "Bead IDs":{"type":"rich_text","rich_text":[{"plain_text":"tut-42"}]}
      }}
    ]}`)

	views := decode(t, `{"results":[
      {"id":"view-1","url":"https://notion/view-1","properties":{
        "View":{"type":"title","title":[{"plain_text":"Messages tab"}]},
        "Route":{"type":"rich_text","rich_text":[{"plain_text":"/students/[id]?tab=messages"}]},
        "Domain":{"type":"select","select":{"name":"Website & Public Presence"}},
        "Spec File":{"type":"rich_text","rich_text":[{"plain_text":"docs/specs/x/messages.md"}]},
        "Deliverables":{"type":"relation","relation":[{"id":"del-1"}]}
      }}
    ]}`)

	g, rep := BuildGraph(caps, delivs, views)

	// Counts.
	if got := len(g.Capabilities); got != 2 {
		t.Errorf("capabilities = %d, want 2", got)
	}
	if got := len(g.Deliverables); got != 1 {
		t.Errorf("deliverables = %d, want 1", got)
	}
	if got := len(g.Views); got != 1 {
		t.Errorf("views = %d, want 1", got)
	}

	// Capability mapping (cap-1 is first).
	c := g.Capabilities[0]
	if c.Title != "Lessons" || c.Level != "capability" || c.DomainSlug != "learning" {
		t.Errorf("capability cap-1 mismapped: %+v", c)
	}
	if c.ParentSourceID != "cap-0" {
		t.Errorf("capability parent = %q, want cap-0", c.ParentSourceID)
	}
	if len(c.MilestoneSlugs) != 2 || c.MilestoneSlugs[0] != "M1" {
		t.Errorf("capability milestones = %v", c.MilestoneSlugs)
	}

	// Deliverable mapping: "—" status → proposed; "Yes" → yes; bead id captured.
	d := g.Deliverables[0]
	if d.Status != "proposed" {
		t.Errorf("deliverable status = %q, want proposed (— placeholder)", d.Status)
	}
	if d.AIReady != "yes" {
		t.Errorf("deliverable ai_ready = %q, want yes", d.AIReady)
	}
	if d.Size != "S" || d.MilestoneSlug != "M3" {
		t.Errorf("deliverable size/milestone mismapped: %+v", d)
	}
	if d.BeadIDs != "tut-42" {
		t.Errorf("deliverable bead ids = %q, want tut-42", d.BeadIDs)
	}
	if len(d.BlockedBySourceIDs) != 1 || d.BlockedBySourceIDs[0] != "del-2" {
		t.Errorf("deliverable blocked-by = %v", d.BlockedBySourceIDs)
	}

	// View mapping: slugified domain, route, spec file.
	v := g.Views[0]
	if v.DomainSlug != "website-public-presence" {
		t.Errorf("view domain slug = %q, want website-public-presence", v.DomainSlug)
	}
	if v.Route == "" || v.SpecFile == "" {
		t.Errorf("view route/spec file empty: %+v", v)
	}

	// Domains: learning + website-public-presence (sorted, deduped from cap+view).
	if len(g.Domains) != 2 {
		t.Errorf("domains = %d (%v), want 2", len(g.Domains), g.Domains)
	}
	if g.Domains[0].Slug != "learning" {
		t.Errorf("domains not sorted: %v", g.Domains)
	}

	// Milestones: M1, M2 (from capability) + M3 (from deliverable), sorted & deduped.
	if len(g.Milestones) != 3 {
		t.Errorf("milestones = %d (%v), want 3", len(g.Milestones), g.Milestones)
	}

	// Findings: a "blocked by" target (del-2) absent from the set is flagged.
	if !hasFinding(rep, "dangling-dependency") {
		t.Errorf("expected a dangling-dependency finding; got %+v", rep.Findings)
	}
}

func TestBuildGraphFallbackDomain(t *testing.T) {
	caps := decode(t, `{"results":[
      {"id":"c1","properties":{
        "Capability":{"type":"title","title":[{"plain_text":"No domain"}]},
        "Level":{"type":"select","select":{"name":"epic"}}
      }}
    ]}`)
	g, rep := BuildGraph(caps, nil, nil)
	if g.Capabilities[0].DomainSlug != "unassigned" {
		t.Errorf("missing domain should fall back to 'unassigned', got %q", g.Capabilities[0].DomainSlug)
	}
	if len(g.Domains) != 1 || g.Domains[0].Slug != "unassigned" {
		t.Errorf("fallback domain not added to graph: %v", g.Domains)
	}
	if !hasFinding(rep, "domain-missing") {
		t.Errorf("expected a domain-missing finding")
	}
}

func TestNormalizers(t *testing.T) {
	for in, want := range map[string]string{"S": "S", "m": "M", "": "", "XXL": ""} {
		if got := normSize(in); got != want {
			t.Errorf("normSize(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"—": "proposed", "": "proposed", "ship": "ship", "Built": "built"} {
		if got := normStatus(in); got != want {
			t.Errorf("normStatus(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"Yes": "yes", "No": "no", "N/A": "na", "": ""} {
		if got := normAIReady(in); got != want {
			t.Errorf("normAIReady(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"Website & Public Presence": "website-public-presence", "Learning": "learning", "  A  B ": "a-b"} {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
