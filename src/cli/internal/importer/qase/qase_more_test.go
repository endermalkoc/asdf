package qase

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---- flexEnum / flexBool decoding ----------------------------------------

func TestFlexEnum_UnmarshalJSON(t *testing.T) {
	// Int on the wire.
	var fi flexEnum
	if err := json.Unmarshal([]byte(`5`), &fi); err != nil {
		t.Fatalf("int: %v", err)
	}
	if !fi.IsInt || fi.Int != 5 || fi.Str != "" {
		t.Fatalf("int decode: %+v", fi)
	}

	// String on the wire.
	var fs flexEnum
	if err := json.Unmarshal([]byte(`"high"`), &fs); err != nil {
		t.Fatalf("str: %v", err)
	}
	if fs.IsInt || fs.Str != "high" {
		t.Fatalf("str decode: %+v", fs)
	}

	// null and empty leave the zero value (via direct call to hit the trim branch).
	var fn flexEnum
	if err := fn.UnmarshalJSON([]byte(` null `)); err != nil {
		t.Fatalf("null: %v", err)
	}
	if fn.IsInt || fn.Str != "" {
		t.Fatalf("null should be zero: %+v", fn)
	}
	var fe flexEnum
	if err := fe.UnmarshalJSON([]byte(``)); err != nil {
		t.Fatalf("empty: %v", err)
	}
	if fe.IsInt || fe.Str != "" {
		t.Fatalf("empty should be zero: %+v", fe)
	}
}

func TestFlexBool_UnmarshalJSON(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{`1`, true},
		{`0`, false},
		{`true`, true},
		{`false`, false},
		{`"true"`, true},
		{`"false"`, false},
		{` "1" `, true},
	}
	for _, tc := range cases {
		var b flexBool
		if err := b.UnmarshalJSON([]byte(tc.in)); err != nil {
			t.Fatalf("unmarshal %q: %v", tc.in, err)
		}
		if bool(b) != tc.want {
			t.Errorf("flexBool(%q) = %v, want %v", tc.in, bool(b), tc.want)
		}
	}
}

// ---- enum maps + normalization edge cases --------------------------------

func TestNormEnum_intAndStringPaths(t *testing.T) {
	// int path looks up enumMaps; string path lowercases & normalizes dashes.
	if got := normEnum(flexEnum{Int: 2, IsInt: true}, enumMaps.layer); got != "unit" {
		t.Errorf("layer int 2 → %q, want unit", got)
	}
	if got := normEnum(flexEnum{Int: 7, IsInt: true}, enumMaps.typ); got != "acceptance" {
		t.Errorf("type int 7 → %q, want acceptance", got)
	}
	if got := normEnum(flexEnum{Int: 2, IsInt: true}, enumMaps.runStatus); got != "aborted" {
		t.Errorf("runStatus int 2 → %q, want aborted", got)
	}
	// Unknown int → "".
	if got := normEnum(flexEnum{Int: 99, IsInt: true}, enumMaps.severity); got != "" {
		t.Errorf("severity int 99 → %q, want empty", got)
	}
	// String path normalizes.
	if got := normEnum(flexEnum{Str: "To-Be-Automated"}, enumMaps.automation); got != "to_be_automated" {
		t.Errorf("string norm → %q", got)
	}
}

func TestNormPriority(t *testing.T) {
	must := func(p *int) int {
		t.Helper()
		if p == nil {
			t.Fatalf("expected non-nil priority")
		}
		return *p
	}
	// int in range passes through.
	if got := must(normPriority(flexEnum{Int: 3, IsInt: true})); got != 3 {
		t.Errorf("int 3 → %d", got)
	}
	// int out of range → nil.
	if p := normPriority(flexEnum{Int: 0, IsInt: true}); p != nil {
		t.Errorf("int 0 → %v, want nil", *p)
	}
	if p := normPriority(flexEnum{Int: 5, IsInt: true}); p != nil {
		t.Errorf("int 5 → %v, want nil", *p)
	}
	// string mappings.
	if got := must(normPriority(flexEnum{Str: "medium"})); got != 2 {
		t.Errorf("medium → %d", got)
	}
	if got := must(normPriority(flexEnum{Str: "Low"})); got != 3 {
		t.Errorf("Low → %d", got)
	}
	if got := must(normPriority(flexEnum{Str: "High"})); got != 1 {
		t.Errorf("High → %d", got)
	}
	// unknown string → nil.
	if p := normPriority(flexEnum{Str: "bogus"}); p != nil {
		t.Errorf("bogus → %v, want nil", *p)
	}
}

// ---- frKey regex extraction ----------------------------------------------

func TestFrCoverage_extractsAcrossFields(t *testing.T) {
	c := caseWire{
		Title:         "check ADDR-FR-001",
		Description:   "also X9-FR-42 here",
		Preconditions: "precond ADDR-FR-001 dup",
		Tags:          []tagWire{{Title: "BILL-FR-007"}},
		CustomFields:  []customFieldWire{{Title: "refs", Value: "see PAY2-FR-3"}},
		Steps:         []stepWire{{Action: "do FOO-FR-99", ExpectedResult: "nope not-fr-1 lower"}},
	}
	got := frCoverage(c)
	want := []string{"ADDR-FR-001", "X9-FR-42", "BILL-FR-007", "PAY2-FR-3", "FOO-FR-99"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	set := map[string]bool{}
	for _, k := range got {
		set[k] = true
	}
	for _, w := range want {
		if !set[w] {
			t.Errorf("missing %q in %v", w, got)
		}
	}
}

func TestFrCoverage_noKeys(t *testing.T) {
	if got := frCoverage(caseWire{Title: "nothing here", Description: "lowercase fr-1 ignored"}); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// ---- BuildGraph: case without suite emits a warning ----------------------

func TestBuildGraph_caseWithoutSuiteWarns(t *testing.T) {
	cases := []caseWire{{ID: 1, Title: "orphan", SuiteID: nil}}
	g, rep := BuildGraph(nil, cases, nil, nil, nil)
	if g.TestCases[0].SuiteSourceID != "" {
		t.Fatalf("expected empty suite id")
	}
	found := false
	for _, f := range rep.Findings {
		if f.Severity == "warn" && strings.Contains(f.Message, "no suite") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a warn finding for suiteless case, got %+v", rep.Findings)
	}
	if rep.Counts["test_cases"] != 1 {
		t.Errorf("count: %d", rep.Counts["test_cases"])
	}
}

// ---- applyDefaults --------------------------------------------------------

func TestApplyDefaults(t *testing.T) {
	c := Config{}
	c.applyDefaults()
	if c.BaseURL != defaultBaseURL {
		t.Errorf("BaseURL default = %q", c.BaseURL)
	}
	if c.HTTPClient == nil || c.HTTPClient.Timeout != 60*time.Second {
		t.Errorf("HTTPClient default not set: %+v", c.HTTPClient)
	}

	// Explicit values are preserved.
	custom := &http.Client{Timeout: time.Second}
	c2 := Config{BaseURL: "http://x", HTTPClient: custom}
	c2.applyDefaults()
	if c2.BaseURL != "http://x" || c2.HTTPClient != custom {
		t.Errorf("explicit values overwritten: %+v", c2)
	}
}

// ---- Parse (HTTP): required-field errors ---------------------------------

func TestParse_requiredFields(t *testing.T) {
	if _, _, err := Parse(context.Background(), Config{Project: "DEMO"}); err == nil {
		t.Error("missing token should error")
	}
	if _, _, err := Parse(context.Background(), Config{Token: "t"}); err == nil {
		t.Error("missing project should error")
	}
}

func qaseTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Token") != "tok" {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		body := ""
		switch {
		case strings.HasPrefix(r.URL.Path, "/suite/DEMO"):
			body = `{"status":true,"result":{"total":1,"count":1,"entities":[{"id":1,"title":"Auth"}]}}`
		case strings.HasPrefix(r.URL.Path, "/case/DEMO"):
			body = `{"status":true,"result":{"total":1,"count":1,"entities":[` +
				`{"id":10,"title":"sign in ADDR-FR-001","suite_id":1,"priority":1,"layer":"e2e",` +
				`"automation":1,"status":0,"is_flaky":1,"tags":[{"title":"BILL-FR-007"}],` +
				`"steps":[{"position":1,"action":"go","expected_result":"ok"}]}]}}`
		case strings.HasPrefix(r.URL.Path, "/configuration/DEMO"):
			body = `{"status":true,"result":{"total":1,"count":1,"entities":[{"title":"Browser","configurations":[{"id":100,"title":"Chrome"}]}]}}`
		case strings.HasPrefix(r.URL.Path, "/run/DEMO"):
			body = `{"status":true,"result":{"total":1,"count":1,"entities":[{"id":50,"title":"Nightly","status":1,"milestone_slug":"M1","configurations":[100]}]}}`
		case strings.HasPrefix(r.URL.Path, "/result/DEMO"):
			body = `{"status":true,"result":{"total":1,"count":1,"entities":[{"run_id":50,"case_id":10,"configuration":{"id":100},"status":"passed","time_spent_ms":1200,"member":"alice"}]}}`
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, body)
	}))
}

func TestParse_httpEndToEnd(t *testing.T) {
	srv := qaseTestServer(t)
	defer srv.Close()

	g, rep, err := Parse(context.Background(), Config{Token: "tok", Project: "DEMO", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(g.TestSuites) != 1 || g.TestSuites[0].Name != "Auth" {
		t.Fatalf("suites: %+v", g.TestSuites)
	}
	c := g.TestCases[0]
	if c.SuiteSourceID != "1" || c.Layer != "e2e" || c.Automation != "automated" || c.Status != "active" || !c.IsFlaky {
		t.Fatalf("case: %+v", c)
	}
	if c.Priority == nil || *c.Priority != 1 {
		t.Fatalf("priority: %v", c.Priority)
	}
	if len(c.FRKeys) != 2 { // ADDR-FR-001 (title) + BILL-FR-007 (tag)
		t.Fatalf("fr keys: %v", c.FRKeys)
	}
	if len(g.Configurations) != 1 || g.Configurations[0].Name != "Chrome" {
		t.Fatalf("configs: %+v", g.Configurations)
	}
	if g.TestRuns[0].Status != "complete" || g.TestRuns[0].MilestoneSlug != "M1" || len(g.TestRuns[0].ConfigSourceIDs) != 1 {
		t.Fatalf("run: %+v", g.TestRuns[0])
	}
	res := g.TestResults[0]
	if res.Status != "passed" || res.ConfigSourceID != "100" || res.ExecutedBy != "alice" {
		t.Fatalf("result: %+v", res)
	}
	if rep.Counts["test_results"] != 1 || rep.Counts["fr_citations"] != 2 {
		t.Fatalf("counts: %+v", rep.Counts)
	}
}

func TestParse_httpErrorPropagates(t *testing.T) {
	// A server that always 500s → the first fetch (suites) fails.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, _, err := Parse(context.Background(), Config{Token: "tok", Project: "DEMO", BaseURL: srv.URL})
	if err == nil {
		t.Fatal("expected an error from a 500 response")
	}
	if !strings.Contains(err.Error(), "qase API") {
		t.Errorf("error should mention the API path: %v", err)
	}
}

// ---- ParseDir / readEntities ---------------------------------------------

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestParseDir_envelopeAndBareArray(t *testing.T) {
	dir := t.TempDir()
	// Envelope form.
	writeFile(t, dir, "suites.json", `{"result":{"entities":[{"id":1,"title":"Auth"},{"id":2,"title":"Login","parent_id":1}]}}`)
	// Bare-array form for cases.
	writeFile(t, dir, "cases.json", `[{"id":10,"title":"sign in ADDR-FR-001","suite_id":2,"priority":"high","severity":4,"type":"functional","layer":"e2e","automation":1,"status":"active","is_flaky":true,"steps":[{"position":1,"action":"a","expected_result":"b"}]}]`)
	writeFile(t, dir, "configurations.json", `{"result":{"entities":[{"title":"Browser","configurations":[{"id":100,"title":"Chrome"}]}]}}`)
	writeFile(t, dir, "runs.json", `{"result":{"entities":[{"id":50,"title":"Nightly","status":"complete","configurations":[100]}]}}`)
	writeFile(t, dir, "results.json", `[{"run_id":50,"case_id":10,"configuration":{"id":100},"status":"passed","time_spent_ms":1200}]`)

	g, rep, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("ParseDir: %v", err)
	}
	if len(g.TestSuites) != 2 || g.TestSuites[1].ParentSourceID != "1" {
		t.Fatalf("suites: %+v", g.TestSuites)
	}
	c := g.TestCases[0]
	if c.Severity != "normal" || c.Priority == nil || *c.Priority != 1 || c.Type != "functional" {
		t.Fatalf("case: %+v", c)
	}
	if len(c.FRKeys) != 1 || c.FRKeys[0] != "ADDR-FR-001" {
		t.Fatalf("fr keys: %v", c.FRKeys)
	}
	if len(g.Configurations) != 1 || len(g.TestRuns) != 1 || len(g.TestResults) != 1 {
		t.Fatalf("graph shape: %+v", g)
	}
	if rep.Counts["test_suites"] != 2 || rep.Counts["test_cases"] != 1 {
		t.Fatalf("counts: %+v", rep.Counts)
	}
}

func TestParseDir_missingFilesAreEmpty(t *testing.T) {
	dir := t.TempDir() // no files at all
	g, rep, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("ParseDir on empty dir: %v", err)
	}
	if len(g.TestSuites)+len(g.TestCases)+len(g.Configurations)+len(g.TestRuns)+len(g.TestResults) != 0 {
		t.Fatalf("expected an empty graph, got %+v", g)
	}
	if rep.Counts["test_cases"] != 0 {
		t.Errorf("counts should be zero: %+v", rep.Counts)
	}
}

func TestParseDir_malformedJSONErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "suites.json", `{"result": not json`)
	_, _, err := ParseDir(dir)
	if err == nil {
		t.Fatal("expected a JSON parse error")
	}
	if !strings.Contains(err.Error(), "suites.json") {
		t.Errorf("error should name the file: %v", err)
	}
}

func TestReadEntities_readErrorNotNotExist(t *testing.T) {
	dir := t.TempDir()
	// A directory named like the target file makes os.ReadFile fail with a
	// non-IsNotExist error, exercising that branch.
	if err := os.Mkdir(filepath.Join(dir, "suites.json"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := readEntities[suiteWire](dir, "suites.json")
	if err == nil {
		t.Fatal("expected a read error for a directory")
	}
}
