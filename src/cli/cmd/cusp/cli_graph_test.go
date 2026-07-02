package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// In-process CLI tests for the graph + query command group: `edge` (add/ls/delete), `check`,
// `impact`, and the read verbs `sql`/`stats`/`search`/`log`. Everything runs against one real
// Dolt workspace per test (each ~6s to start), so the tests sweep many related commands on a
// single server. Shared setup + runCLI live in cli_harness_test.go; naming is TestCLI_Graph_*.

// graphImpactLink / graphImpact mirror app.ImpactReport for parsing `impact --json`.
type graphImpactLink struct {
	Endpoint string `json:"endpoint"`
	Via      string `json:"via"`
}

type graphImpact struct {
	Subject    string            `json:"subject"`
	Inbound    []graphImpactLink `json:"inbound"`
	Outbound   []graphImpactLink `json:"outbound"`
	Transitive []string          `json:"transitive"`
}

// graphAddReq adds a requirement to the given spec prefix and returns its derived fr_key,
// parsed from the --json emit. Fails the test on a nonzero exit or unparseable output.
func graphAddReq(t *testing.T, prefix, statement string) string {
	t.Helper()
	out, code := runCLI(t, "--json", "req", "add", prefix, statement)
	if code != 0 {
		t.Fatalf("req add %q exit=%d out=%s", statement, code, out)
	}
	var r struct {
		FRKey string `json:"fr_key"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("req add --json unparseable: %v\n%s", err, out)
	}
	if r.FRKey == "" {
		t.Fatalf("req add returned empty fr_key: %s", out)
	}
	return r.FRKey
}

// graphJSONLen unmarshals s as a JSON array of objects and returns its length.
func graphJSONLen(t *testing.T, s string) int {
	t.Helper()
	var arr []map[string]any
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		t.Fatalf("expected a JSON array, got err=%v:\n%s", err, s)
	}
	return len(arr)
}

// TestCLI_Graph_Sweep drives the full graph + query surface on one workspace: it builds a
// domain → spec → three requirements, links them with typed edges, then exercises edge
// add/ls/delete, impact, check (clean + dangling), and the sql/stats/search/log read verbs,
// covering both happy paths and the validation (exit 2) / not_found (exit 3) / generic (exit 1)
// error paths.
func TestCLI_Graph_Sweep(t *testing.T) {
	newCLIWorkspace(t)

	// --- Section A: prerequisites (domain → spec-with-prefix → three requirements) ---------
	if _, code := runCLI(t, "domain", "add", "core", "Core"); code != 0 {
		t.Fatalf("domain add exit=%d", code)
	}
	if _, code := runCLI(t, "spec", "add", "att.md", "--domain", "core", "--prefix", "ATT", "--title", "Attendance"); code != 0 {
		t.Fatalf("spec add exit=%d", code)
	}
	fr1 := graphAddReq(t, "ATT", "first requirement about attendance")
	fr2 := graphAddReq(t, "ATT", "second requirement about attendance")
	fr3 := graphAddReq(t, "ATT", "third requirement about attendance")

	// --- Section B: edge add (bare fr_key + TYPE:key forms) + ls -----------------------------
	// 002 refines 001 (bare-value endpoints are taken as requirement fr_keys).
	out, code := runCLI(t, "--json", "edge", "add", fr2, "refines", fr1)
	if code != 0 {
		t.Fatalf("edge add refines exit=%d out=%s", code, out)
	}
	var added struct {
		ID   string `json:"id"`
		From string `json:"from"`
		Kind string `json:"kind"`
		To   string `json:"to"`
	}
	if err := json.Unmarshal([]byte(out), &added); err != nil {
		t.Fatalf("edge add --json unparseable: %v\n%s", err, out)
	}
	if added.ID == "" || added.From != "requirement:"+fr2 || added.To != "requirement:"+fr1 || added.Kind != "refines" {
		t.Fatalf("edge add json mismatch: %+v", added)
	}
	// 003 depends_on 001 via explicit REQ: prefixes.
	if _, code := runCLI(t, "edge", "add", "REQ:"+fr3, "depends_on", "REQ:"+fr1); code != 0 {
		t.Fatalf("edge add depends_on exit=%d", code)
	}
	// Re-adding the same edge is a no-op with the same deterministic id.
	out, code = runCLI(t, "--json", "edge", "add", fr2, "refines", fr1)
	if code != 0 {
		t.Fatalf("edge re-add exit=%d", code)
	}
	var readd struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(out), &readd)
	if readd.ID != added.ID {
		t.Fatalf("edge re-add expected same id %q, got %q", added.ID, readd.ID)
	}
	// ls (human) shows the resolved endpoints + kind.
	out, code = runCLI(t, "edge", "ls")
	if code != 0 || !strings.Contains(out, "refines") || !strings.Contains(out, fr2) {
		t.Fatalf("edge ls exit=%d out=%s", code, out)
	}
	// ls --json is a JSON array of both edges.
	out, code = runCLI(t, "--json", "edge", "ls")
	if code != 0 || !validJSON(out) {
		t.Fatalf("edge ls --json exit=%d valid=%v", code, validJSON(out))
	}
	if n := graphJSONLen(t, out); n != 2 {
		t.Fatalf("expected 2 edges, got %d\n%s", n, out)
	}

	// --- Section C: impact -------------------------------------------------------------------
	// fr1 is pointed at by both fr2 (refines) and fr3 (depends_on): two inbound links.
	out, code = runCLI(t, "impact", "REQ:"+fr1)
	if code != 0 || !strings.Contains(out, "impact of requirement:"+fr1) || !strings.Contains(out, fr2) || !strings.Contains(out, fr3) {
		t.Fatalf("impact fr1 exit=%d out=%s", code, out)
	}
	out, code = runCLI(t, "--json", "impact", fr1)
	if code != 0 {
		t.Fatalf("impact fr1 --json exit=%d", code)
	}
	var rep graphImpact
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("impact --json unparseable: %v\n%s", err, out)
	}
	if rep.Subject != "requirement:"+fr1 || len(rep.Inbound) != 2 {
		t.Fatalf("impact fr1 report mismatch: %+v", rep)
	}
	// --transitive adds the reverse-edge closure: fr2 and fr3 both reach fr1.
	out, code = runCLI(t, "--json", "impact", fr1, "--transitive")
	if code != 0 {
		t.Fatalf("impact --transitive exit=%d", code)
	}
	var trep graphImpact
	if err := json.Unmarshal([]byte(out), &trep); err != nil {
		t.Fatalf("impact --transitive unparseable: %v\n%s", err, out)
	}
	if len(trep.Transitive) != 2 {
		t.Fatalf("expected 2 transitive dependents, got %v", trep.Transitive)
	}
	// fr2 points outbound at fr1 (via refines).
	out, code = runCLI(t, "--json", "impact", "REQ:"+fr2)
	if code != 0 {
		t.Fatalf("impact fr2 --json exit=%d", code)
	}
	var rep2 graphImpact
	if err := json.Unmarshal([]byte(out), &rep2); err != nil {
		t.Fatalf("impact fr2 unparseable: %v\n%s", err, out)
	}
	if len(rep2.Outbound) != 1 || rep2.Outbound[0].Via != "refines" {
		t.Fatalf("impact fr2 outbound mismatch: %+v", rep2.Outbound)
	}

	// --- Section D: check (clean graph) ------------------------------------------------------
	out, code = runCLI(t, "check")
	if code != 0 || !strings.Contains(out, "no integrity issues") {
		t.Fatalf("check clean exit=%d out=%s", code, out)
	}
	out, code = runCLI(t, "--json", "check")
	if code != 0 || !validJSON(out) {
		t.Fatalf("check --json exit=%d valid=%v out=%s", code, validJSON(out), out)
	}
	if n := graphJSONLen(t, out); n != 0 {
		t.Fatalf("expected 0 findings on a clean graph, got %d\n%s", n, out)
	}

	// --- Section E: query verbs (stats / sql / search / log) ---------------------------------
	out, code = runCLI(t, "--json", "stats")
	if code != 0 {
		t.Fatalf("stats --json exit=%d", code)
	}
	var stats []map[string]any
	if err := json.Unmarshal([]byte(out), &stats); err != nil {
		t.Fatalf("stats --json unparseable: %v\n%s", err, out)
	}
	if got := statCount(stats, "edges"); got != "2" {
		t.Fatalf("expected 2 edges in stats, got %q", got)
	}
	if got := statCount(stats, "requirements"); got != "3" {
		t.Fatalf("expected 3 requirements in stats, got %q", got)
	}
	if out, code := runCLI(t, "stats"); code != 0 || !strings.Contains(out, "edges") {
		t.Fatalf("stats (human) exit=%d out=%s", code, out)
	}
	// sql: read-only SELECT (human table + --json array), plus a trivial SELECT.
	if out, code := runCLI(t, "sql", "SELECT COUNT(*) AS n FROM req_requirement"); code != 0 || !strings.Contains(out, "3") {
		t.Fatalf("sql count exit=%d out=%s", code, out)
	}
	out, code = runCLI(t, "--json", "sql", "SELECT fr_key FROM req_requirement ORDER BY fr_key")
	if code != 0 || !validJSON(out) {
		t.Fatalf("sql --json exit=%d valid=%v", code, validJSON(out))
	}
	if n := graphJSONLen(t, out); n != 3 {
		t.Fatalf("expected 3 rows from sql, got %d\n%s", n, out)
	}
	if _, code := runCLI(t, "sql", "SELECT 1"); code != 0 {
		t.Fatalf("sql SELECT 1 exit=%d", code)
	}
	// search: a matching term and a --json result set.
	if _, code := runCLI(t, "search", "attendance"); code != 0 {
		t.Fatalf("search attendance exit=%d", code)
	}
	out, code = runCLI(t, "--json", "search", "attendance")
	if code != 0 || !validJSON(out) {
		t.Fatalf("search --json exit=%d valid=%v", code, validJSON(out))
	}
	if n := graphJSONLen(t, out); n == 0 {
		t.Fatalf("expected search hits for 'attendance', got none\n%s", out)
	}
	// a no-match search still exits 0 (emits null/empty under --json).
	if out, code := runCLI(t, "--json", "search", "zzznomatchzzz"); code != 0 || !validJSON(out) {
		t.Fatalf("search no-match exit=%d valid=%v out=%s", code, validJSON(out), out)
	}
	// log: Dolt history on the active branch (human table + --json).
	if out, code := runCLI(t, "log"); code != 0 || !strings.Contains(out, "message") {
		t.Fatalf("log (human) exit=%d out=%s", code, out)
	}
	if out, code := runCLI(t, "--json", "log", "--limit", "5"); code != 0 || !validJSON(out) {
		t.Fatalf("log --json exit=%d valid=%v", code, validJSON(out))
	}

	// --- Section F: edge delete --------------------------------------------------------------
	if out, code := runCLI(t, "edge", "delete", fr2, "refines", fr1); code != 0 || !strings.Contains(out, "deleted edge") {
		t.Fatalf("edge delete exit=%d out=%s", code, out)
	}
	out, code = runCLI(t, "--json", "edge", "ls")
	if code != 0 {
		t.Fatalf("edge ls after delete exit=%d", code)
	}
	if n := graphJSONLen(t, out); n != 1 {
		t.Fatalf("expected 1 edge after delete, got %d\n%s", n, out)
	}
	// deleting an edge that no longer exists is not_found (3).
	if _, code := runCLI(t, "edge", "delete", fr2, "refines", fr1); code != 3 {
		t.Fatalf("re-delete expected not_found 3, got %d", code)
	}

	// --- Section G: error paths --------------------------------------------------------------
	// invalid edge kind → validation (2).
	if _, code := runCLI(t, "edge", "add", fr1, "bogus_kind", fr2); code != 2 {
		t.Fatalf("bad edge kind expected 2, got %d", code)
	}
	// unknown endpoints → not_found (3), both directions.
	if _, code := runCLI(t, "edge", "add", "REQ:NOPE-FR-999", "refines", fr1); code != 3 {
		t.Fatalf("unknown from-endpoint expected 3, got %d", code)
	}
	if _, code := runCLI(t, "edge", "add", fr1, "refines", "REQ:NOPE-FR-999"); code != 3 {
		t.Fatalf("unknown to-endpoint expected 3, got %d", code)
	}
	// a self-loop on an acyclic kind → generic (1).
	if _, code := runCLI(t, "edge", "add", fr1, "refines", fr1); code != 1 {
		t.Fatalf("self-loop expected 1, got %d", code)
	}
	// closing a cycle (fr3 depends_on fr1 already exists) → generic (1).
	if _, code := runCLI(t, "edge", "add", fr1, "depends_on", fr3); code != 1 {
		t.Fatalf("cycle expected 1, got %d", code)
	}
	// edge delete with an unknown endpoint → not_found (3).
	if _, code := runCLI(t, "edge", "delete", "REQ:NOPE-FR-999", "refines", fr1); code != 3 {
		t.Fatalf("delete unknown endpoint expected 3, got %d", code)
	}
	// impact of a missing entity → not_found (3), both TYPE:key and bare forms.
	if _, code := runCLI(t, "impact", "REQ:NOPE-FR-999"); code != 3 {
		t.Fatalf("impact unknown expected 3, got %d", code)
	}
	if _, code := runCLI(t, "impact", "NOPE-FR-999"); code != 3 {
		t.Fatalf("impact bare unknown expected 3, got %d", code)
	}
	// sql write verbs are rejected → validation (2).
	if _, code := runCLI(t, "sql", "UPDATE req_requirement SET statement='x' WHERE 1=0"); code != 2 {
		t.Fatalf("sql UPDATE expected 2, got %d", code)
	}
	if _, code := runCLI(t, "sql", "DELETE FROM req_requirement"); code != 2 {
		t.Fatalf("sql DELETE expected 2, got %d", code)
	}
	// a read against a missing table surfaces as a generic error (1).
	if _, code := runCLI(t, "sql", "SELECT * FROM no_such_table_xyz"); code != 1 {
		t.Fatalf("sql bad table expected 1, got %d", code)
	}

	// --- Section H: check surfaces a dangling reference (nonzero exit) -----------------------
	// Seed a requirement whose statement points at an entity that does not exist. --force lets
	// the write through despite the dangling token; check must then report it and exit nonzero.
	if _, code := runCLI(t, "req", "add", "ATT", "refer to [[ENTITY:Ghost]] for details", "--force"); code != 0 {
		t.Fatalf("req add --force (dangling) exit=%d", code)
	}
	out, code = runCLI(t, "check")
	if code != 1 || !strings.Contains(out, "dangling-ref") || !strings.Contains(out, "ENTITY:Ghost") {
		t.Fatalf("check dangling exit=%d out=%s", code, out)
	}
	out, code = runCLI(t, "--json", "check")
	if code != 1 {
		t.Fatalf("check dangling --json exit=%d out=%s", code, out)
	}
	// Under --json a failing command prints the findings array (via emit) AND then the error
	// envelope (via Execute) to stdout — two JSON documents. Decode just the first (the findings).
	var findings []struct {
		Kind   string `json:"kind"`
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&findings); err != nil {
		t.Fatalf("check --json findings unparseable: %v\n%s", err, out)
	}
	if len(findings) != 1 || findings[0].Kind != "dangling-ref" || !strings.Contains(findings[0].Detail, "ENTITY:Ghost") {
		t.Fatalf("check dangling findings mismatch: %+v", findings)
	}
}
