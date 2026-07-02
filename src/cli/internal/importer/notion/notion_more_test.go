package notion

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Config.applyDefaults --------------------------------------------------

func TestApplyDefaultsFillsBlanks(t *testing.T) {
	var c Config
	c.applyDefaults()
	if c.Version != defaultAPIVersion {
		t.Errorf("Version = %q, want %q", c.Version, defaultAPIVersion)
	}
	if c.CapabilitiesDB != DefaultCapabilitiesDB {
		t.Errorf("CapabilitiesDB = %q, want default", c.CapabilitiesDB)
	}
	if c.DeliverablesDB != DefaultDeliverablesDB {
		t.Errorf("DeliverablesDB = %q, want default", c.DeliverablesDB)
	}
	if c.ViewsDB != DefaultViewsDB {
		t.Errorf("ViewsDB = %q, want default", c.ViewsDB)
	}
	if c.HTTPClient == nil {
		t.Errorf("HTTPClient should be non-nil after applyDefaults")
	}
}

func TestApplyDefaultsPreservesExplicit(t *testing.T) {
	custom := &http.Client{}
	c := Config{
		Version:        "2099-01-01",
		CapabilitiesDB: "cap-db",
		DeliverablesDB: "del-db",
		ViewsDB:        "view-db",
		HTTPClient:     custom,
	}
	c.applyDefaults()
	if c.Version != "2099-01-01" || c.CapabilitiesDB != "cap-db" ||
		c.DeliverablesDB != "del-db" || c.ViewsDB != "view-db" {
		t.Errorf("applyDefaults clobbered explicit values: %+v", c)
	}
	if c.HTTPClient != custom {
		t.Errorf("applyDefaults replaced an explicit HTTPClient")
	}
}

// --- round-tripper plumbing for the API source -----------------------------

// rtFunc adapts a function into an http.RoundTripper so tests can intercept the
// (hardcoded) api.notion.com requests fetchAll makes.
type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func dbIDFromPath(p string) string {
	// /v1/databases/<id>/query
	p = strings.TrimPrefix(p, "/v1/databases/")
	return strings.TrimSuffix(p, "/query")
}

// --- Parse (happy path, with pagination) -----------------------------------

func TestParseHappyPathWithPagination(t *testing.T) {
	capsPage1 := `{"results":[
      {"id":"cap-1","url":"u","properties":{
        "Capability":{"type":"title","title":[{"plain_text":"Lessons"}]},
        "Domain":{"type":"select","select":{"name":"Learning"}}
      }}
    ],"has_more":true,"next_cursor":"cursor2"}`
	capsPage2 := `{"results":[
      {"id":"cap-2","url":"u","properties":{
        "Capability":{"type":"title","title":[{"plain_text":"Grading"}]},
        "Domain":{"type":"select","select":{"name":"Learning"}}
      }}
    ],"has_more":false,"next_cursor":""}`
	delivsBody := `{"results":[
      {"id":"del-1","url":"u","properties":{
        "Deliverable":{"type":"title","title":[{"plain_text":"Reminder"}]},
        "Status":{"type":"select","select":{"name":"built"}}
      }}
    ]}`
	viewsBody := `{"results":[]}`

	var capCalls int
	transport := rtFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing/incorrect Authorization header: %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Notion-Version") == "" {
			t.Errorf("missing Notion-Version header")
		}
		body, _ := io.ReadAll(r.Body)
		hasCursor := strings.Contains(string(body), "start_cursor")
		switch dbIDFromPath(r.URL.Path) {
		case DefaultCapabilitiesDB:
			capCalls++
			if hasCursor {
				return jsonResp(http.StatusOK, capsPage2), nil
			}
			return jsonResp(http.StatusOK, capsPage1), nil
		case DefaultDeliverablesDB:
			return jsonResp(http.StatusOK, delivsBody), nil
		case DefaultViewsDB:
			return jsonResp(http.StatusOK, viewsBody), nil
		}
		t.Fatalf("unexpected path %q", r.URL.Path)
		return nil, nil
	})

	cfg := Config{Token: "tok", HTTPClient: &http.Client{Transport: transport}}
	g, rep, err := Parse(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if capCalls != 2 {
		t.Errorf("expected 2 capability page fetches (pagination), got %d", capCalls)
	}
	if len(g.Capabilities) != 2 {
		t.Errorf("capabilities = %d, want 2 (both pages)", len(g.Capabilities))
	}
	if len(g.Deliverables) != 1 {
		t.Errorf("deliverables = %d, want 1", len(g.Deliverables))
	}
	if rep.Counts["capabilities"] != 2 {
		t.Errorf("report capability count = %d, want 2", rep.Counts["capabilities"])
	}
}

func TestParseRequiresToken(t *testing.T) {
	_, _, err := Parse(context.Background(), Config{Token: "   "})
	if err == nil || !strings.Contains(err.Error(), "token is required") {
		t.Fatalf("expected token-required error, got %v", err)
	}
}

// TestParseWrapsFetchError confirms a fetch failure on the first database is
// surfaced with the "fetch capabilities" prefix.
func TestParseWrapsFetchError(t *testing.T) {
	transport := rtFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusUnauthorized, `{"message":"unauthorized"}`), nil
	})
	cfg := Config{Token: "tok", HTTPClient: &http.Client{Transport: transport}}
	_, _, err := Parse(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "fetch capabilities") {
		t.Fatalf("expected wrapped fetch-capabilities error, got %v", err)
	}
}

// --- fetchAll error/edge branches ------------------------------------------

func TestFetchAllErrorWithMessage(t *testing.T) {
	transport := rtFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusUnauthorized, `{"message":"API token is invalid"}`), nil
	})
	c := &Config{Token: "tok", Version: defaultAPIVersion, HTTPClient: &http.Client{Transport: transport}}
	_, err := c.fetchAll(context.Background(), "db")
	if err == nil || !strings.Contains(err.Error(), "API token is invalid") {
		t.Fatalf("expected API message in error, got %v", err)
	}
}

func TestFetchAllErrorWithoutMessage(t *testing.T) {
	transport := rtFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusInternalServerError, `{}`), nil
	})
	c := &Config{Token: "tok", Version: defaultAPIVersion, HTTPClient: &http.Client{Transport: transport}}
	_, err := c.fetchAll(context.Background(), "db")
	if err == nil || !strings.Contains(err.Error(), "notion API") {
		t.Fatalf("expected bare notion API error, got %v", err)
	}
	if strings.Contains(err.Error(), ":") && strings.Count(err.Error(), ":") > 1 {
		// A bare error is "notion API <status>" — no trailing message segment.
		t.Errorf("unexpected message segment in bare error: %v", err)
	}
}

func TestFetchAllTransportError(t *testing.T) {
	transport := rtFunc(func(r *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("dial tcp: connection refused")
	})
	c := &Config{Token: "tok", Version: defaultAPIVersion, HTTPClient: &http.Client{Transport: transport}}
	_, err := c.fetchAll(context.Background(), "db")
	if err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("expected transport error to propagate, got %v", err)
	}
}

func TestFetchAllDecodeError(t *testing.T) {
	transport := rtFunc(func(r *http.Request) (*http.Response, error) {
		// 200 OK but a body that cannot decode into the page struct.
		return jsonResp(http.StatusOK, `{"results": "not-an-array"}`), nil
	})
	c := &Config{Token: "tok", Version: defaultAPIVersion, HTTPClient: &http.Client{Transport: transport}}
	_, err := c.fetchAll(context.Background(), "db")
	if err == nil {
		t.Fatalf("expected a decode error for a malformed 200 body")
	}
}

// --- ParseDir / readPages --------------------------------------------------

const (
	capsFile   = `{"results":[{"id":"cap-1","properties":{"Capability":{"type":"title","title":[{"plain_text":"Lessons"}]},"Domain":{"type":"select","select":{"name":"Learning"}}}}]}`
	delivsFile = `[{"id":"del-1","properties":{"Deliverable":{"type":"title","title":[{"plain_text":"Reminder"}]},"Status":{"type":"select","select":{"name":"specced"}}}}]`
	viewsFile  = `{"results":[{"id":"view-1","properties":{"View":{"type":"title","title":[{"plain_text":"Tab"}]},"Domain":{"type":"select","select":{"name":"Learning"}}}}]}`
)

func writeDir(t *testing.T, caps, delivs, views *string) string {
	t.Helper()
	dir := t.TempDir()
	if caps != nil {
		if err := os.WriteFile(filepath.Join(dir, "capabilities.json"), []byte(*caps), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if delivs != nil {
		if err := os.WriteFile(filepath.Join(dir, "deliverables.json"), []byte(*delivs), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if views != nil {
		if err := os.WriteFile(filepath.Join(dir, "views.json"), []byte(*views), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestParseDirSuccess(t *testing.T) {
	c, d, v := capsFile, delivsFile, viewsFile
	dir := writeDir(t, &c, &d, &v)
	g, rep, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("ParseDir: %v", err)
	}
	if len(g.Capabilities) != 1 || len(g.Deliverables) != 1 || len(g.Views) != 1 {
		t.Fatalf("unexpected graph sizes: caps=%d delivs=%d views=%d",
			len(g.Capabilities), len(g.Deliverables), len(g.Views))
	}
	// The bare-array deliverables file must decode too (readPages fallback path).
	if g.Deliverables[0].Status != "specced" {
		t.Errorf("bare-array deliverable status = %q, want specced", g.Deliverables[0].Status)
	}
	if rep.Counts["capabilities"] != 1 {
		t.Errorf("report capability count = %d, want 1", rep.Counts["capabilities"])
	}
}

func TestParseDirMissingFiles(t *testing.T) {
	c, d, v := capsFile, delivsFile, viewsFile

	// Missing capabilities.json → error from the first readPages.
	if _, _, err := ParseDir(writeDir(t, nil, &d, &v)); err == nil {
		t.Errorf("expected error when capabilities.json is missing")
	}
	// Missing deliverables.json → error from the second readPages.
	if _, _, err := ParseDir(writeDir(t, &c, nil, &v)); err == nil {
		t.Errorf("expected error when deliverables.json is missing")
	}
	// Missing views.json → error from the third readPages.
	if _, _, err := ParseDir(writeDir(t, &c, &d, nil)); err == nil {
		t.Errorf("expected error when views.json is missing")
	}
}

func TestReadPagesBareArray(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bare.json")
	if err := os.WriteFile(path, []byte(`[{"id":"x"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	pages, err := readPages(path)
	if err != nil {
		t.Fatalf("readPages(bare array): %v", err)
	}
	if len(pages) != 1 || pages[0].ID != "x" {
		t.Errorf("bare-array decode = %+v", pages)
	}
}

func TestReadPagesWrapped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wrapped.json")
	if err := os.WriteFile(path, []byte(`{"results":[{"id":"y"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	pages, err := readPages(path)
	if err != nil {
		t.Fatalf("readPages(wrapped): %v", err)
	}
	if len(pages) != 1 || pages[0].ID != "y" {
		t.Errorf("wrapped decode = %+v", pages)
	}
}

func TestReadPagesInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	// A JSON string: neither a {"results":…} object nor a page array.
	if err := os.WriteFile(path, []byte(`"just a string"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readPages(path); err == nil {
		t.Fatalf("expected a parse error for non-page JSON")
	}
}

func TestReadPagesMissingFile(t *testing.T) {
	if _, err := readPages(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatalf("expected an error reading a missing file")
	}
}

// --- small mapping edges ---------------------------------------------------

func TestTitleAnyNoTitleProperty(t *testing.T) {
	p := Page{Properties: map[string]Property{
		"Status": {Type: "select", Select: &selectOption{Name: "built"}},
	}}
	if got := p.titleAny(); got != "" {
		t.Errorf("titleAny with no title property = %q, want empty", got)
	}
}

func TestNormStatusPassthroughAndKnown(t *testing.T) {
	cases := map[string]string{
		"proposed": "proposed",
		"specced":  "specced",
		"wired":    "wired",
		"built":    "built",
		"ship":     "ship",
		"–":        "proposed", // en-dash placeholder
		"-":        "proposed", // hyphen placeholder
		"WIP":      "wip",      // unknown → lowercased passthrough
	}
	for in, want := range cases {
		if got := normStatus(in); got != want {
			t.Errorf("normStatus(%q) = %q, want %q", in, got, want)
		}
	}
}
