package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// In-process CLI tests for the structure command layer: domain, spec, req (+tree), entity,
// term (glossary), and section/section-type. Each test starts a fresh workspace via the shared
// harness (newCLIWorkspace) and sweeps many related commands over that one Dolt server, asserting
// exit codes (0 ok / 2 validation / 3 not_found / 4 dangling) and key output. Shared helpers
// (newCLIWorkspace, runCLI, validJSON) live in cli_harness_test.go.

// structureResetTree resets every flag in the whole command tree to un-Changed + its default
// value before each command. This is REQUIRED because cobra's per-command Changed() state and the
// bound package vars PERSIST across in-process Execute() calls (the shared harness only clears the
// global persistent flags). Without it, e.g. a prior `req add ... --priority 99` leaves
// Changed("priority")==true and reqPriority==99, so a later plain `req add` would re-validate the
// stale 99 and fail — cross-test contamination (all groups merge into one package) makes this
// order-dependent. (Note: pflag's per-flagset actual/NFlag() count is monotonic and unexported, so
// this cannot clear it; the `NFlag()==0` "nothing to edit" guard is therefore not asserted here.)
func structureResetTree() {
	reset := func(f *pflag.Flag) {
		f.Changed = false
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			_ = sv.Replace(nil)
			return
		}
		_ = f.Value.Set(f.DefValue)
	}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.Flags().VisitAll(reset)
		c.PersistentFlags().VisitAll(reset)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(rootCmd)
}

// structureRun resets the command-tree flags (see structureResetTree) then drives one command via
// the shared runCLI. Use it for every call so each command sees a clean, fresh-process flag slate.
func structureRun(t *testing.T, args ...string) (string, int) {
	t.Helper()
	structureResetTree()
	return runCLI(t, args...)
}

// structureWantCode fails (non-fatally) if got != want, logging the command and its output.
func structureWantCode(t *testing.T, label string, got, want int, out string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: exit=%d want=%d\n%s", label, got, want, out)
	}
}

// TestCLI_Structure_DomainSpecReqTree sweeps domain + spec + req + req tree + priority + the
// entity read/error surface (entities are import-created — there is no `entity add` — so only the
// read + not-found paths are reachable from the CLI).
func TestCLI_Structure_DomainSpecReqTree(t *testing.T) {
	newCLIWorkspace(t)

	// --- domain: add / ls / show / edit (partial) / delete + error paths ---
	if out, code := structureRun(t, "domain", "add", "core", "Core Domain", "--description", "the core things"); code != 0 {
		t.Fatalf("domain add core: exit=%d\n%s", code, out)
	} else if !strings.Contains(out, "core") {
		t.Fatalf("domain add core: output missing slug\n%s", out)
	}
	// wrong arg count (cobra ExactArgs) is a generic (non-zero) error.
	if _, code := structureRun(t, "domain", "add", "onlyslug"); code == 0 {
		t.Errorf("domain add with 1 arg should fail")
	}
	if out, code := structureRun(t, "domain", "ls"); code != 0 || !strings.Contains(out, "core") {
		t.Errorf("domain ls: exit=%d out=%s", code, out)
	}
	if out, code := structureRun(t, "--json", "domain", "ls"); code != 0 || !validJSON(out) {
		t.Errorf("domain ls --json: exit=%d valid=%v", code, validJSON(out))
	}
	if out, code := structureRun(t, "domain", "show", "core"); code != 0 || !strings.Contains(out, "Core Domain") {
		t.Errorf("domain show core: exit=%d out=%s", code, out)
	}
	if out, code := structureRun(t, "--json", "domain", "show", "core"); code != 0 || !validJSON(out) {
		t.Errorf("domain show core --json: exit=%d valid=%v", code, validJSON(out))
	}
	_, code := structureRun(t, "domain", "show", "nope")
	structureWantCode(t, "domain show missing", code, 3, "")
	// partial-flag edits.
	if _, code := structureRun(t, "domain", "edit", "core", "--name", "Core Renamed"); code != 0 {
		t.Errorf("domain edit --name: exit=%d", code)
	}
	if _, code := structureRun(t, "domain", "edit", "core", "--status", "active"); code != 0 {
		t.Errorf("domain edit --status active: exit=%d", code)
	}
	_, code = structureRun(t, "domain", "edit", "core", "--status", "bogus")
	structureWantCode(t, "domain edit bad status", code, 2, "")
	_, code = structureRun(t, "domain", "edit", "nope", "--name", "X")
	structureWantCode(t, "domain edit missing", code, 3, "")
	if out, code := structureRun(t, "domain", "show", "core"); code != 0 || !strings.Contains(out, "Core Renamed") {
		t.Errorf("domain show after edit: name not updated\n%s", out)
	}
	// delete a throwaway domain (keep core for the spec tests below).
	if _, code := structureRun(t, "domain", "add", "temp", "Temp"); code != 0 {
		t.Fatalf("domain add temp failed")
	}
	if _, code := structureRun(t, "domain", "delete", "temp"); code != 0 {
		t.Errorf("domain delete temp: exit=%d", code)
	}
	_, code = structureRun(t, "domain", "delete", "temp")
	structureWantCode(t, "domain delete missing", code, 3, "")

	// --- spec: add (with + without prefix) / show / render / edit / delete + errors ---
	if out, code := structureRun(t, "spec", "add", "student/add.md", "--domain", "core", "--prefix", "STU", "--title", "Add a student to a class"); code != 0 || !strings.Contains(out, "STU") {
		t.Fatalf("spec add STU: exit=%d out=%s", code, out)
	}
	// missing required --domain → non-zero (cobra required-flag error).
	if _, code := structureRun(t, "spec", "add", "orphan.md", "--prefix", "X", "--title", "Orphan"); code == 0 {
		t.Errorf("spec add without --domain should fail")
	}
	// FR-exempt spec (no prefix).
	if out, code := structureRun(t, "spec", "add", "notes.md", "--domain", "core", "--title", "Loose notes"); code != 0 {
		t.Errorf("spec add no-prefix: exit=%d out=%s", code, out)
	}
	if out, code := structureRun(t, "spec", "show", "STU"); code != 0 || !strings.Contains(out, "STU") {
		t.Errorf("spec show STU: exit=%d out=%s", code, out)
	}
	if out, code := structureRun(t, "--json", "spec", "show", "STU"); code != 0 || !validJSON(out) {
		t.Errorf("spec show STU --json: exit=%d valid=%v", code, validJSON(out))
	}
	_, code = structureRun(t, "spec", "show", "NOPE")
	structureWantCode(t, "spec show missing", code, 3, "")
	if out, code := structureRun(t, "spec", "render", "STU", "--format", "md"); code != 0 {
		t.Errorf("spec render STU --format md: exit=%d out=%s", code, out)
	}
	if _, code := structureRun(t, "spec", "edit", "STU", "--title", "Add a student (revised)"); code != 0 {
		t.Errorf("spec edit --title: exit=%d", code)
	}
	_, code = structureRun(t, "spec", "edit", "STU", "--status", "bogus")
	structureWantCode(t, "spec edit bad status", code, 2, "")
	_, code = structureRun(t, "spec", "edit", "NOPE", "--title", "X")
	structureWantCode(t, "spec edit missing", code, 3, "")
	// add a throwaway spec (with a prefix so it's addressable) and delete it; keep STU.
	if _, code := structureRun(t, "spec", "add", "temp.md", "--domain", "core", "--prefix", "TMP", "--title", "Temp spec"); code != 0 {
		t.Fatalf("spec add TMP failed")
	}
	if _, code := structureRun(t, "spec", "delete", "TMP"); code != 0 {
		t.Errorf("spec delete TMP: exit=%d", code)
	}
	_, code = structureRun(t, "spec", "delete", "TMP")
	structureWantCode(t, "spec delete missing", code, 3, "")

	// --- req: add / ls / show / edit / delete + tree + priority + error paths ---
	frKey1 := structureAddReq(t, "STU", "An admin must be able to add a student to a class roster")
	if _, code := structureRun(t, "req", "add", "STU", "The roster must reject a duplicate student", "--delivery", "covered"); code != 0 {
		t.Errorf("req add --delivery: exit=%d", code)
	}
	if _, code := structureRun(t, "req", "add", "STU", "A student must have a display name", "--priority", "0"); code != 0 {
		t.Errorf("req add --priority 0: exit=%d", code)
	}
	frKeyDel := structureAddReq(t, "STU", "A throwaway requirement to delete")
	// error adds.
	_, code = structureRun(t, "req", "add", "STU", "strict bad delivery", "--delivery", "totally-bogus", "--strict")
	structureWantCode(t, "req add strict bad delivery", code, 2, "")
	_, code = structureRun(t, "req", "add", "STU", "bad priority", "--priority", "99")
	structureWantCode(t, "req add bad priority", code, 1, "")
	_, code = structureRun(t, "req", "add", "STU", "dangling ref to [[REQ:ZZZ-FR-999]]")
	structureWantCode(t, "req add dangling (no force)", code, 4, "")
	if _, code := structureRun(t, "req", "add", "STU", "dangling ref to [[REQ:ZZZ-FR-998]]", "--force"); code != 0 {
		t.Errorf("req add dangling --force: exit=%d", code)
	}

	if out, code := structureRun(t, "req", "ls", "STU"); code != 0 || !strings.Contains(out, frKey1) {
		t.Errorf("req ls STU: exit=%d out=%s", code, out)
	}
	if out, code := structureRun(t, "--json", "req", "ls", "STU"); code != 0 || !validJSON(out) {
		t.Errorf("req ls STU --json: exit=%d valid=%v", code, validJSON(out))
	}
	if out, code := structureRun(t, "req", "show", frKey1); code != 0 || !strings.Contains(out, frKey1) {
		t.Errorf("req show %s: exit=%d out=%s", frKey1, code, out)
	}
	if out, code := structureRun(t, "--json", "req", "show", frKey1); code != 0 || !validJSON(out) {
		t.Errorf("req show --json: exit=%d valid=%v", code, validJSON(out))
	}
	_, code = structureRun(t, "req", "show", "STU-FR-999")
	structureWantCode(t, "req show missing", code, 3, "")
	// partial-flag edits.
	if _, code := structureRun(t, "req", "edit", frKey1, "--notes", "reviewed with product"); code != 0 {
		t.Errorf("req edit --notes: exit=%d", code)
	}
	if _, code := structureRun(t, "req", "edit", frKey1, "--delivery", "covered"); code != 0 {
		t.Errorf("req edit --delivery: exit=%d", code)
	}
	if _, code := structureRun(t, "req", "edit", frKey1, "--content-status", "active"); code != 0 {
		t.Errorf("req edit --content-status: exit=%d", code)
	}
	if _, code := structureRun(t, "req", "edit", frKey1, "--statement", "An admin adds a student to the class roster"); code != 0 {
		t.Errorf("req edit --statement: exit=%d", code)
	}
	_, code = structureRun(t, "req", "edit", frKey1, "--content-status", "bogus")
	structureWantCode(t, "req edit bad content-status", code, 2, "")
	_, code = structureRun(t, "req", "edit", frKey1, "--priority", "99")
	structureWantCode(t, "req edit bad priority", code, 1, "")
	_, code = structureRun(t, "req", "edit", "STU-FR-999", "--notes", "x")
	structureWantCode(t, "req edit missing", code, 3, "")
	// delete.
	if _, code := structureRun(t, "req", "delete", frKeyDel); code != 0 {
		t.Errorf("req delete %s: exit=%d", frKeyDel, code)
	}
	_, code = structureRun(t, "req", "delete", frKeyDel)
	structureWantCode(t, "req delete missing", code, 3, "")

	// give STU a prose section so the tree exercises the "Other" (sections) branch + rendering.
	if _, code := structureRun(t, "spec", "section-type", "add", "context", "--title", "Context", "--position", "20"); code != 0 {
		t.Errorf("spec section-type add context: exit=%d", code)
	}
	if _, code := structureRun(t, "spec", "section", "add", "STU", "--type", "context", "--body", "Background context for the student spec."); code != 0 {
		t.Errorf("spec section add STU/context: exit=%d", code)
	}

	// --- req tree (text + json) ---
	if out, code := structureRun(t, "req", "tree"); code != 0 || !strings.Contains(out, "STU") || !strings.Contains(out, "Context") {
		t.Errorf("req tree: exit=%d out=%s", code, out)
	}
	out, code := structureRun(t, "--json", "req", "tree")
	if code != 0 || !validJSON(out) {
		t.Fatalf("req tree --json: exit=%d valid=%v", code, validJSON(out))
	}
	var tree []treeDomain
	if err := json.Unmarshal([]byte(out), &tree); err != nil {
		t.Fatalf("req tree --json unmarshal: %v\n%s", err, out)
	}
	foundSTU := false
	for _, d := range tree {
		for _, s := range d.Specs {
			if s.Prefix == "STU" {
				foundSTU = true
			}
		}
	}
	if !foundSTU {
		t.Errorf("req tree --json: STU spec not present\n%s", out)
	}

	// --- priority ls ---
	if out, code := structureRun(t, "priority", "ls"); code != 0 || !strings.Contains(out, "0") {
		t.Errorf("priority ls: exit=%d out=%s", code, out)
	}
	if out, code := structureRun(t, "--json", "priority", "ls"); code != 0 || !validJSON(out) {
		t.Errorf("priority ls --json: exit=%d valid=%v", code, validJSON(out))
	}

	// --- entity: read + error surface (no `entity add` — entities are import-created) ---
	if out, code := structureRun(t, "entity", "ls"); code != 0 {
		t.Errorf("entity ls (empty): exit=%d out=%s", code, out)
	}
	if out, code := structureRun(t, "--json", "entity", "ls"); code != 0 || !validJSON(out) {
		t.Errorf("entity ls --json: exit=%d valid=%v", code, validJSON(out))
	}
	if out, code := structureRun(t, "entity", "tree"); code != 0 {
		t.Errorf("entity tree (empty): exit=%d out=%s", code, out)
	}
	if out, code := structureRun(t, "--json", "entity", "tree"); code != 0 || !validJSON(out) {
		t.Errorf("entity tree --json: exit=%d valid=%v", code, validJSON(out))
	}
	_, code = structureRun(t, "entity", "show", "Ghost")
	structureWantCode(t, "entity show missing", code, 3, "")
	_, code = structureRun(t, "entity", "edit", "Ghost", "--status", "active")
	structureWantCode(t, "entity edit missing", code, 3, "")
	_, code = structureRun(t, "entity", "delete", "Ghost")
	structureWantCode(t, "entity delete missing", code, 3, "")
	_, code = structureRun(t, "entity", "render", "Ghost")
	structureWantCode(t, "entity render missing", code, 3, "")
	_, code = structureRun(t, "entity", "render", "Ghost", "--format", "md")
	structureWantCode(t, "entity render missing (md)", code, 3, "")
}

// structureAddReq adds a requirement to specPrefix and returns its derived fr_key (fatal on error).
func structureAddReq(t *testing.T, specPrefix, statement string) string {
	t.Helper()
	out, code := structureRun(t, "--json", "req", "add", specPrefix, statement)
	if code != 0 {
		t.Fatalf("req add %s: exit=%d\n%s", specPrefix, code, out)
	}
	var r struct {
		FRKey string `json:"fr_key"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil || r.FRKey == "" {
		t.Fatalf("req add: no fr_key (err=%v)\n%s", err, out)
	}
	return r.FRKey
}

// TestCLI_Structure_Term sweeps the glossary: add (with name/alias/domain/status) / ls / show /
// edit (partial) / delete, plus the validation (bad status), not_found, and dangling-ref paths.
func TestCLI_Structure_Term(t *testing.T) {
	newCLIWorkspace(t)

	// happy add (slug defaults the display name).
	if out, code := structureRun(t, "term", "add", "makeup", "A make-up session for a missed class"); code != 0 || !strings.Contains(out, "makeup") {
		t.Fatalf("term add makeup: exit=%d out=%s", code, out)
	}
	// add with name + repeatable alias + status.
	if _, code := structureRun(t, "term", "add", "sla", "A service level agreement", "--name", "SLA", "--alias", "service-level", "--alias", "svc-level", "--status", "active"); code != 0 {
		t.Errorf("term add sla with flags: exit=%d", code)
	}
	// bad status is a hard validation error.
	_, code := structureRun(t, "term", "add", "bad", "x", "--status", "bogus")
	structureWantCode(t, "term add bad status", code, 2, "")
	// scope to a domain: create one, then a scoped term; an unknown domain is not_found.
	if _, code := structureRun(t, "domain", "add", "core", "Core"); code != 0 {
		t.Fatalf("domain add core failed")
	}
	if _, code := structureRun(t, "term", "add", "scoped", "A domain-scoped term", "--domain", "core"); code != 0 {
		t.Errorf("term add --domain core: exit=%d", code)
	}
	_, code = structureRun(t, "term", "add", "scoped2", "def", "--domain", "no-such-domain")
	structureWantCode(t, "term add unknown domain", code, 3, "")
	// dangling ref in the definition blocks the write unless --force.
	_, code = structureRun(t, "term", "add", "dang", "refers to [[REQ:ZZZ-FR-001]]")
	structureWantCode(t, "term add dangling (no force)", code, 4, "")
	if _, code := structureRun(t, "term", "add", "dang", "refers to [[REQ:ZZZ-FR-001]]", "--force"); code != 0 {
		t.Errorf("term add dangling --force: exit=%d", code)
	}

	// ls / show.
	if out, code := structureRun(t, "term", "ls"); code != 0 || !strings.Contains(out, "makeup") {
		t.Errorf("term ls: exit=%d out=%s", code, out)
	}
	if out, code := structureRun(t, "--json", "term", "ls"); code != 0 || !validJSON(out) {
		t.Errorf("term ls --json: exit=%d valid=%v", code, validJSON(out))
	}
	if out, code := structureRun(t, "term", "show", "makeup"); code != 0 || !strings.Contains(out, "make-up") {
		t.Errorf("term show makeup: exit=%d out=%s", code, out)
	}
	if out, code := structureRun(t, "--json", "term", "show", "sla"); code != 0 || !validJSON(out) {
		t.Errorf("term show sla --json: exit=%d valid=%v", code, validJSON(out))
	}
	_, code = structureRun(t, "term", "show", "nope")
	structureWantCode(t, "term show missing", code, 3, "")

	// partial-flag edits.
	if _, code := structureRun(t, "term", "edit", "makeup", "--name", "Make-up session"); code != 0 {
		t.Errorf("term edit --name: exit=%d", code)
	}
	if _, code := structureRun(t, "term", "edit", "makeup", "--definition", "A rescheduled session for a missed class"); code != 0 {
		t.Errorf("term edit --definition: exit=%d", code)
	}
	_, code = structureRun(t, "term", "edit", "makeup", "--status", "bogus")
	structureWantCode(t, "term edit bad status", code, 2, "")
	_, code = structureRun(t, "term", "edit", "nope", "--name", "X")
	structureWantCode(t, "term edit missing", code, 3, "")
	// editing a definition re-validates its inline links (dangling blocks unless --force).
	_, code = structureRun(t, "term", "edit", "sla", "--definition", "now cites [[REQ:ZZZ-FR-777]]")
	structureWantCode(t, "term edit dangling def", code, 4, "")
	if _, code := structureRun(t, "term", "edit", "sla", "--definition", "now cites [[REQ:ZZZ-FR-777]]", "--force"); code != 0 {
		t.Errorf("term edit dangling def --force: exit=%d", code)
	}

	// delete.
	if _, code := structureRun(t, "term", "delete", "makeup"); code != 0 {
		t.Errorf("term delete makeup: exit=%d", code)
	}
	_, code = structureRun(t, "term", "delete", "makeup")
	structureWantCode(t, "term delete missing", code, 3, "")
}

// TestCLI_Structure_Section sweeps the spec + entity section / section-type command trees:
// section-type add/ls, section add/ls/delete, and their error paths (missing --type, unknown
// type, unknown owner, deleting an absent section).
func TestCLI_Structure_Section(t *testing.T) {
	newCLIWorkspace(t)

	// prerequisites: a domain and a spec (owner of the sections).
	if _, code := structureRun(t, "domain", "add", "core", "Core"); code != 0 {
		t.Fatalf("domain add core failed")
	}
	if _, code := structureRun(t, "spec", "add", "main.md", "--domain", "core", "--prefix", "SEC", "--title", "Section spec"); code != 0 {
		t.Fatalf("spec add SEC failed")
	}

	// --- spec section-type: add / ls ---
	if out, code := structureRun(t, "spec", "section-type", "ls"); code != 0 {
		t.Errorf("spec section-type ls (initial): exit=%d out=%s", code, out)
	}
	if out, code := structureRun(t, "--json", "spec", "section-type", "ls"); code != 0 || !validJSON(out) {
		t.Errorf("spec section-type ls --json: exit=%d valid=%v", code, validJSON(out))
	}
	if out, code := structureRun(t, "spec", "section-type", "add", "background", "--title", "Background", "--position", "10", "--level", "2", "--description", "context for the spec"); code != 0 || !strings.Contains(out, "background") {
		t.Fatalf("spec section-type add background: exit=%d out=%s", code, out)
	}
	if _, code := structureRun(t, "spec", "section-type", "add", "overview", "--title", "Overview", "--position", "5"); code != 0 {
		t.Errorf("spec section-type add overview: exit=%d", code)
	}
	if out, code := structureRun(t, "spec", "section-type", "ls"); code != 0 || !strings.Contains(out, "background") {
		t.Errorf("spec section-type ls: exit=%d out=%s", code, out)
	}

	// --- spec section: add / ls / delete + errors ---
	if _, code := structureRun(t, "spec", "section", "add", "SEC", "--type", "background", "--body", "Some background prose."); code != 0 {
		t.Errorf("spec section add background: exit=%d", code)
	}
	// a long, multi-line body exercises firstLine's newline-split + truncation in `section ls`.
	if _, code := structureRun(t, "spec", "section", "add", "SEC", "--type", "overview", "--body", "This is a deliberately long overview paragraph that runs well past seventy characters\nand carries a second line to exercise the newline split."); code != 0 {
		t.Errorf("spec section add overview: exit=%d", code)
	}
	// missing --type.
	_, code := structureRun(t, "spec", "section", "add", "SEC", "--body", "no type")
	structureWantCode(t, "spec section add no --type", code, 1, "")
	// unknown type (friction: must create the type first).
	_, code = structureRun(t, "spec", "section", "add", "SEC", "--type", "nonexistent", "--body", "x")
	structureWantCode(t, "spec section add unknown type", code, 1, "")
	// unknown owner spec.
	_, code = structureRun(t, "spec", "section", "add", "NOPE", "--type", "background", "--body", "x")
	structureWantCode(t, "spec section add unknown spec", code, 3, "")

	if out, code := structureRun(t, "spec", "section", "ls", "SEC"); code != 0 || !strings.Contains(out, "background") {
		t.Errorf("spec section ls SEC: exit=%d out=%s", code, out)
	}
	if out, code := structureRun(t, "--json", "spec", "section", "ls", "SEC"); code != 0 || !validJSON(out) {
		t.Errorf("spec section ls SEC --json: exit=%d valid=%v", code, validJSON(out))
	}
	_, code = structureRun(t, "spec", "section", "ls", "NOPE")
	structureWantCode(t, "spec section ls unknown spec", code, 3, "")

	// delete: succeed, then deleting the now-absent section is not_found.
	if _, code := structureRun(t, "spec", "section", "delete", "SEC", "--type", "background"); code != 0 {
		t.Errorf("spec section delete background: exit=%d", code)
	}
	_, code = structureRun(t, "spec", "section", "delete", "SEC", "--type", "background")
	structureWantCode(t, "spec section delete absent", code, 3, "")
	_, code = structureRun(t, "spec", "section", "delete", "SEC")
	structureWantCode(t, "spec section delete no --type", code, 1, "")
	_, code = structureRun(t, "spec", "section", "delete", "NOPE", "--type", "overview")
	structureWantCode(t, "spec section delete unknown spec", code, 3, "")

	// --- entity section / section-type: the second namespace (entities are import-created, so
	// only the section-type verbs and the unknown-owner error paths are reachable here). ---
	if out, code := structureRun(t, "entity", "section-type", "add", "traits", "--title", "Traits", "--position", "3"); code != 0 || !strings.Contains(out, "traits") {
		t.Errorf("entity section-type add: exit=%d out=%s", code, out)
	}
	if out, code := structureRun(t, "entity", "section-type", "ls"); code != 0 || !strings.Contains(out, "traits") {
		t.Errorf("entity section-type ls: exit=%d out=%s", code, out)
	}
	_, code = structureRun(t, "entity", "section", "add", "Ghost", "--type", "traits", "--body", "x")
	structureWantCode(t, "entity section add unknown entity", code, 3, "")
	_, code = structureRun(t, "entity", "section", "ls", "Ghost")
	structureWantCode(t, "entity section ls unknown entity", code, 3, "")
}
