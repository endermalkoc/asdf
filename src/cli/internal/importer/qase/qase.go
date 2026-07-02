// Package qase parses a Qase test-management project into Cusp's testing-layer entity
// shapes (importer.Graph). Like the notion adapter it is pure: no DB writes, no minted
// ULIDs — it produces a *Graph + *Report; importer.Apply does the write pass.
//
// Two sources, one mapping (BuildGraph): the Qase API v1 (--token) or saved API responses
// in a directory (--from, offline/testable). Qase encodes case enums as integers; each enum
// field here decodes as int OR string and normalizes string-first, so offline fixtures can
// use readable strings and the integer maps (best-effort, see enumMaps) are the API fallback.
package qase

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/endermalkoc/cusp/internal/importer"
)

const defaultBaseURL = "https://api.qase.io/v1"

// Config drives a Qase API parse.
type Config struct {
	Token      string // Qase API token (required for Parse)
	Project    string // Qase project code, e.g. "DEMO" (required)
	BaseURL    string // default https://api.qase.io/v1
	HTTPClient *http.Client
}

func (c *Config) applyDefaults() {
	if c.BaseURL == "" {
		c.BaseURL = defaultBaseURL
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
}

// ---- wire types (the Qase API subset we read) -----------------------------

type listEnvelope[T any] struct {
	Status bool `json:"status"`
	Result struct {
		Total    int `json:"total"`
		Count    int `json:"count"`
		Entities []T `json:"entities"`
	} `json:"result"`
}

type suiteWire struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ParentID    *int   `json:"parent_id"`
	Position    *int   `json:"position"`
}

type stepWire struct {
	Position       *int   `json:"position"`
	Action         string `json:"action"`
	ExpectedResult string `json:"expected_result"`
}

type customFieldWire struct {
	Title string `json:"title"`
	Value string `json:"value"`
}

// tagWire is a Qase case tag; the FR ids the qase-sync skill attaches live in the title.
type tagWire struct {
	Title string `json:"title"`
}

type caseWire struct {
	ID            int               `json:"id"`
	Title         string            `json:"title"`
	Description   string            `json:"description"`
	Preconditions string            `json:"preconditions"`
	SuiteID       *int              `json:"suite_id"`
	Priority      flexEnum          `json:"priority"`
	Severity      flexEnum          `json:"severity"`
	Type          flexEnum          `json:"type"`
	Layer         flexEnum          `json:"layer"`
	Automation    flexEnum          `json:"automation"`
	Status        flexEnum          `json:"status"`
	IsFlaky       flexBool          `json:"is_flaky"`
	Automated     string            `json:"path"` // automated test path, when present
	Steps         []stepWire        `json:"steps"`
	Tags          []tagWire         `json:"tags"`
	CustomFields  []customFieldWire `json:"custom_fields"`
}

type configGroupWire struct {
	Title          string `json:"title"`
	Configurations []struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
	} `json:"configurations"`
}

type runWire struct {
	ID          int      `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      flexEnum `json:"status"`
	Milestone   string   `json:"milestone_slug"`
	Configs     []int    `json:"configurations"`
}

type configRef struct {
	ID int `json:"id"`
}

type resultWire struct {
	RunID   int        `json:"run_id"`
	CaseID  int        `json:"case_id"`
	Config  *configRef `json:"configuration"`
	Status  string     `json:"status"`
	Comment string     `json:"comment"`
	TimeMs  *int       `json:"time_spent_ms"`
	Member  string     `json:"member"`
}

// flexEnum decodes a Qase enum field that may be an int (API) or a string (fixtures).
type flexEnum struct {
	Str   string
	Int   int
	IsInt bool
}

func (f *flexEnum) UnmarshalJSON(b []byte) error {
	b = []byte(strings.TrimSpace(string(b)))
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		return json.Unmarshal(b, &f.Str)
	}
	f.IsInt = true
	return json.Unmarshal(b, &f.Int)
}

// flexBool decodes 0/1, true/false, or "true"/"false".
type flexBool bool

func (f *flexBool) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	*f = flexBool(s == "1" || s == "true")
	return nil
}

// ---- public entrypoints ---------------------------------------------------

// Parse fetches a Qase project via the API and builds the Graph.
func Parse(ctx context.Context, cfg Config) (*importer.Graph, *importer.Report, error) {
	cfg.applyDefaults()
	if cfg.Token == "" {
		return nil, nil, fmt.Errorf("a Qase token is required (set --token or CUSP_QASE_TOKEN / QASE_API_TOKEN)")
	}
	if cfg.Project == "" {
		return nil, nil, fmt.Errorf("a Qase project code is required (--project)")
	}
	suites, err := fetchList[suiteWire](ctx, cfg, "/suite/"+cfg.Project)
	if err != nil {
		return nil, nil, err
	}
	cases, err := fetchList[caseWire](ctx, cfg, "/case/"+cfg.Project)
	if err != nil {
		return nil, nil, err
	}
	groups, err := fetchConfigGroups(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	runs, err := fetchList[runWire](ctx, cfg, "/run/"+cfg.Project)
	if err != nil {
		return nil, nil, err
	}
	// Results across all runs come from the project-level list endpoint (each result carries its
	// run_id/case_id); the per-run path treats its last segment as a result hash and 404s.
	results, err := fetchList[resultWire](ctx, cfg, "/result/"+cfg.Project)
	if err != nil {
		return nil, nil, err
	}
	g, rep := BuildGraph(suites, cases, groups, runs, results)
	return g, rep, nil
}

// ParseDir builds the Graph from saved API responses in dir: suites.json / cases.json /
// configurations.json / runs.json / results.json (each a {"result":{"entities":[…]}} envelope
// or a bare array). Missing files are treated as empty.
func ParseDir(dir string) (*importer.Graph, *importer.Report, error) {
	suites, err := readEntities[suiteWire](dir, "suites.json")
	if err != nil {
		return nil, nil, err
	}
	cases, err := readEntities[caseWire](dir, "cases.json")
	if err != nil {
		return nil, nil, err
	}
	groups, err := readEntities[configGroupWire](dir, "configurations.json")
	if err != nil {
		return nil, nil, err
	}
	runs, err := readEntities[runWire](dir, "runs.json")
	if err != nil {
		return nil, nil, err
	}
	results, err := readEntities[resultWire](dir, "results.json")
	if err != nil {
		return nil, nil, err
	}
	g, rep := BuildGraph(suites, cases, groups, runs, results)
	return g, rep, nil
}

// ---- pure mapping ---------------------------------------------------------

var frKeyRe = regexp.MustCompile(`\b[A-Z][A-Z0-9]*-FR-\d+\b`)

// BuildGraph maps the Qase wire subset into importer testing shapes (pure). Source ids are the
// stringified Qase numeric ids.
func BuildGraph(suites []suiteWire, cases []caseWire, groups []configGroupWire, runs []runWire, results []resultWire) (*importer.Graph, *importer.Report) {
	g := &importer.Graph{}
	rep := &importer.Report{Counts: map[string]int{}, Coverage: map[string]int{}}

	for _, s := range suites {
		g.TestSuites = append(g.TestSuites, importer.TestSuite{
			SourceID: id(s.ID), ParentSourceID: idp(s.ParentID), Name: s.Title, Description: s.Description, Position: s.Position,
		})
	}
	for _, c := range cases {
		tc := importer.TestCase{
			SourceID: id(c.ID), SuiteSourceID: idp(c.SuiteID), Title: c.Title, Description: c.Description,
			Preconditions: c.Preconditions, Path: c.Automated, IsFlaky: bool(c.IsFlaky),
			Layer:      normEnum(c.Layer, enumMaps.layer),
			Type:       normEnum(c.Type, enumMaps.typ),
			Severity:   normEnum(c.Severity, enumMaps.severity),
			Automation: normEnum(c.Automation, enumMaps.automation),
			Status:     normEnum(c.Status, enumMaps.caseStatus),
			Priority:   normPriority(c.Priority),
			FRKeys:     frCoverage(c),
		}
		for _, st := range c.Steps {
			tc.Steps = append(tc.Steps, importer.TestStep{Position: st.Position, Action: st.Action, ExpectedResult: st.ExpectedResult})
		}
		if tc.SuiteSourceID == "" {
			rep.Add(importer.SevWarn, "test_case", "case has no suite — will be skipped", tc.Title)
		}
		g.TestCases = append(g.TestCases, tc)
	}
	for _, grp := range groups {
		for _, cf := range grp.Configurations {
			g.Configurations = append(g.Configurations, importer.TestConfiguration{
				SourceID: id(cf.ID), Group: grp.Title, Name: cf.Title,
			})
		}
	}
	for _, r := range runs {
		tr := importer.TestRun{
			SourceID: id(r.ID), Title: r.Title, Description: r.Description, MilestoneSlug: r.Milestone,
			Status: normEnum(r.Status, enumMaps.runStatus),
		}
		for _, ci := range r.Configs {
			tr.ConfigSourceIDs = append(tr.ConfigSourceIDs, id(ci))
		}
		g.TestRuns = append(g.TestRuns, tr)
	}
	for _, res := range results {
		cfg := ""
		if res.Config != nil {
			cfg = id(res.Config.ID)
		}
		g.TestResults = append(g.TestResults, importer.TestResult{
			RunSourceID: id(res.RunID), CaseSourceID: id(res.CaseID), ConfigSourceID: cfg,
			Status: normResultStatus(res.Status), Comment: res.Comment, DurationMs: res.TimeMs, ExecutedBy: res.Member,
		})
	}

	rep.Counts["test_suites"] = len(g.TestSuites)
	rep.Counts["test_cases"] = len(g.TestCases)
	rep.Counts["test_configurations"] = len(g.Configurations)
	rep.Counts["test_runs"] = len(g.TestRuns)
	rep.Counts["test_results"] = len(g.TestResults)
	covered := 0
	for _, c := range g.TestCases {
		covered += len(c.FRKeys)
	}
	rep.Counts["fr_citations"] = covered
	return g, rep
}

// frCoverage collects the fr_key citations from a case's text fields, tags, and custom fields.
func frCoverage(c caseWire) []string {
	var text strings.Builder
	text.WriteString(c.Title + "\n" + c.Description + "\n" + c.Preconditions + "\n")
	for _, t := range c.Tags {
		text.WriteString(t.Title + "\n")
	}
	for _, cf := range c.CustomFields {
		text.WriteString(cf.Value + "\n")
	}
	for _, st := range c.Steps {
		text.WriteString(st.Action + "\n" + st.ExpectedResult + "\n")
	}
	seen := map[string]bool{}
	var out []string
	for _, m := range frKeyRe.FindAllString(text.String(), -1) {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

// ---- enum normalization ---------------------------------------------------

// enumMaps holds the best-effort Qase integer → Cusp label maps (the API encodes these enums as
// ints). A string value on the wire is matched directly against the Cusp set first, so these only
// apply to the API path and are easy to adjust if a Qase project uses different codes.
var enumMaps = struct {
	layer, typ, severity, automation, caseStatus, runStatus map[int]string
}{
	layer:      map[int]string{0: "e2e", 1: "integration", 2: "unit", 3: "component", 4: "shared"},
	typ:        map[int]string{0: "other", 1: "functional", 2: "smoke", 3: "regression", 7: "acceptance"},
	severity:   map[int]string{1: "blocker", 2: "critical", 3: "major", 4: "normal", 5: "minor", 6: "trivial"},
	automation: map[int]string{0: "manual", 1: "automated", 2: "to_be_automated"},
	caseStatus: map[int]string{0: "active", 1: "draft", 2: "deprecated"},
	runStatus:  map[int]string{0: "active", 1: "complete", 2: "aborted"},
}

// normEnum resolves a flexEnum to a Cusp label: a string is lower-cased and returned as-is (the
// store validates it); an int is looked up in the map. Unknowns yield "".
func normEnum(f flexEnum, m map[int]string) string {
	if !f.IsInt {
		return normStr(f.Str)
	}
	return m[f.Int]
}

func normStr(s string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, "-", "_")))
}

// normResultStatus maps a Qase result status (already string-typed in the API) to the Cusp set.
func normResultStatus(s string) string {
	return normStr(s)
}

// normPriority maps a Qase priority to the Cusp 0–4 scheme (pass ints through when in range;
// map the common high/medium/low strings). 0/undefined → nil.
func normPriority(f flexEnum) *int {
	if f.IsInt {
		if f.Int <= 0 || f.Int > 4 {
			return nil
		}
		v := f.Int
		return &v
	}
	m := map[string]int{"high": 1, "medium": 2, "normal": 2, "low": 3}
	if v, ok := m[normStr(f.Str)]; ok {
		return &v
	}
	return nil
}

// ---- helpers --------------------------------------------------------------

func id(n int) string { return fmt.Sprintf("%d", n) }
func idp(p *int) string {
	if p == nil {
		return ""
	}
	return id(*p)
}

// ---- API client (inline, mirroring the notion adapter) --------------------

// fetchList pulls a paginated Qase list endpoint (limit/offset), collecting all entities.
func fetchList[T any](ctx context.Context, cfg Config, path string) ([]T, error) {
	const limit = 100
	var out []T
	for offset := 0; ; offset += limit {
		var env listEnvelope[T]
		if err := cfg.get(ctx, fmt.Sprintf("%s?limit=%d&offset=%d", path, limit, offset), &env); err != nil {
			return nil, err
		}
		out = append(out, env.Result.Entities...)
		if len(env.Result.Entities) < limit || len(out) >= env.Result.Total {
			break
		}
	}
	return out, nil
}

func fetchConfigGroups(ctx context.Context, cfg Config) ([]configGroupWire, error) {
	return fetchList[configGroupWire](ctx, cfg, "/configuration/"+cfg.Project)
}

func (c *Config) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Token", c.Token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("qase API %s: %s: %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, out)
}

// readEntities reads dir/file as either a {"result":{"entities":[…]}} envelope or a bare array.
func readEntities[T any](dir, file string) ([]T, error) {
	b, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	trimmed := strings.TrimSpace(string(b))
	if strings.HasPrefix(trimmed, "[") {
		var arr []T
		return arr, json.Unmarshal(b, &arr)
	}
	var env listEnvelope[T]
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return env.Result.Entities, nil
}
