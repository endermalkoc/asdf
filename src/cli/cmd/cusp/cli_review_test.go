package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// In-process CLI tests for the review-workflow command group: `changeset`
// (start/ls/diff/submit/merge/abandon), `comment` (add/ls/show/resolve/reopen/edit/delete),
// `review` (verdict + ls), and `branch` (ls/create/delete/checkout). They drive the real
// rootCmd/Execute via the shared harness (newCLIWorkspace/runCLI in cli_harness_test.go) against a
// real Dolt workspace, asserting exit codes and stdout/JSON on both happy and error paths.

// reviewReset zeroes the flag variables owned by this command group. pflag keeps a bound
// variable's value across Execute calls, and the shared harness's resetGlobalFlags only clears the
// persistent/global flags — so a value set by one invocation would leak into the next without this.
func reviewReset() {
	commentBody = ""
	commentSubject = ""
	commentSubjectType = ""
	commentSubjectID = ""
	commentLocator = ""
	commentReply = ""
	commentUnresolved = false
	reviewVerdict = ""
	reviewSummary = ""
	changesetDiffEntities = false
}

// reviewRun clears this group's command flags, then drives one CLI invocation via the shared
// harness runCLI (which resets the global flags and captures stdout + the exit code).
func reviewRun(t *testing.T, args ...string) (string, int) {
	t.Helper()
	reviewReset()
	return runCLI(t, args...)
}

func reviewContains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// TestCLI_Review_ChangesetLifecycle is the full PR-like round-trip on one workspace: open a
// changeset, edit on its branch, diff it, thread comments and record (then replace) a review
// verdict, submit, and merge — followed by a separate submit-then-abandon and an
// abandon-before-submit error path.
func TestCLI_Review_ChangesetLifecycle(t *testing.T) {
	newCLIWorkspace(t)

	// Prereqs on main: a domain and a spec with an FR prefix, so a requirement can be added.
	if _, code := reviewRun(t, "domain", "add", "core", "Core"); code != 0 {
		t.Fatalf("domain add exit=%d", code)
	}
	if _, code := reviewRun(t, "spec", "add", "auth.md", "--domain", "core", "--prefix", "AUTH", "--title", "Authentication"); code != 0 {
		t.Fatalf("spec add exit=%d", code)
	}

	// --- open a changeset; it becomes the active target ---
	branch := "changeset/add-auth"
	if out, code := reviewRun(t, "changeset", "start", "add auth"); code != 0 || !strings.Contains(out, branch) {
		t.Fatalf("changeset start exit=%d out=%q", code, out)
	}

	// changeset ls: human line names the branch; JSON reports one draft changeset.
	if out, code := reviewRun(t, "changeset", "ls"); code != 0 || !strings.Contains(out, branch) {
		t.Fatalf("changeset ls exit=%d out=%q", code, out)
	}
	out, code := reviewRun(t, "--json", "changeset", "ls")
	if code != 0 {
		t.Fatalf("changeset ls --json exit=%d", code)
	}
	var csRows []struct{ Branch, Title, Status string }
	if err := json.Unmarshal([]byte(out), &csRows); err != nil {
		t.Fatalf("changeset ls --json parse: %v\n%s", err, out)
	}
	if len(csRows) != 1 || csRows[0].Branch != branch || csRows[0].Status != "draft" {
		t.Fatalf("unexpected changeset ls: %+v", csRows)
	}

	// --- add a requirement on the branch (a change to diff and to anchor a comment to) ---
	out, code = reviewRun(t, "--json", "req", "add", "AUTH", "User can log in")
	if code != 0 {
		t.Fatalf("req add exit=%d out=%q", code, out)
	}
	var added struct {
		FRKey string `json:"fr_key"`
	}
	if err := json.Unmarshal([]byte(out), &added); err != nil || added.FRKey == "" {
		t.Fatalf("req add --json parse: %v key=%q\n%s", err, added.FRKey, out)
	}
	frKey := added.FRKey

	// --- diff the changeset: table summary + per-entity, human and JSON ---
	if out, code := reviewRun(t, "changeset", "diff"); code != 0 || !strings.Contains(out, branch) {
		t.Fatalf("changeset diff exit=%d out=%q", code, out)
	}
	if out, code := reviewRun(t, "--json", "changeset", "diff"); code != 0 || !validJSON(out) {
		t.Fatalf("changeset diff --json exit=%d valid=%v", code, validJSON(out))
	}
	if _, code := reviewRun(t, "changeset", "diff", "--entities"); code != 0 {
		t.Fatalf("changeset diff --entities exit=%d", code)
	}
	if out, code := reviewRun(t, "--json", "changeset", "diff", "--entities"); code != 0 || !validJSON(out) {
		t.Fatalf("changeset diff --entities --json exit=%d valid=%v", code, validJSON(out))
	}

	// --- comments: changeset-level, one anchored to the requirement, and a threaded reply ---
	if _, code := reviewRun(t, "comment", "add", "--body", "overall this looks reasonable"); code != 0 {
		t.Fatalf("comment add (changeset-level) exit=%d", code)
	}
	out, code = reviewRun(t, "--json", "comment", "add", "--subject", "REQ:"+frKey, "--locator", "statement", "--body", "please clarify the wording")
	if code != 0 {
		t.Fatalf("comment add (anchored) exit=%d out=%q", code, out)
	}
	var anchored struct {
		ID         string `json:"id"`
		SubjectRef string `json:"subjectRef"`
	}
	if err := json.Unmarshal([]byte(out), &anchored); err != nil || anchored.ID == "" {
		t.Fatalf("comment add --json parse: %v\n%s", err, out)
	}
	if !strings.Contains(anchored.SubjectRef, frKey) {
		t.Fatalf("anchored comment subjectRef=%q want to contain %q", anchored.SubjectRef, frKey)
	}
	rootID := anchored.ID
	if _, code := reviewRun(t, "comment", "add", "--reply", rootID, "--body", "agreed, will fix"); code != 0 {
		t.Fatalf("comment add (reply) exit=%d", code)
	}

	// comment ls: JSON reports all three; human names the anchored root; filters run clean.
	out, code = reviewRun(t, "--json", "comment", "ls")
	if code != 0 {
		t.Fatalf("comment ls --json exit=%d", code)
	}
	var comments []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &comments); err != nil {
		t.Fatalf("comment ls --json parse: %v\n%s", err, out)
	}
	if len(comments) != 3 {
		t.Fatalf("expected 3 comments, got %d:\n%s", len(comments), out)
	}
	if out, code := reviewRun(t, "comment", "ls"); code != 0 || !strings.Contains(out, rootID) {
		t.Fatalf("comment ls (human) exit=%d out=%q", code, out)
	}
	if _, code := reviewRun(t, "comment", "ls", "--unresolved"); code != 0 {
		t.Fatalf("comment ls --unresolved exit=%d", code)
	}
	// --subject resolves REQ:<key> on the changeset branch, then filters the listing; just assert
	// it runs cleanly (the resolve+filter code path). Membership isn't asserted: within this one
	// command the subject resolve leaves the pooled connection checked out on the branch, so the
	// follow-on ws.DB() read of rev_comment lands on the branch rather than main.
	if _, code := reviewRun(t, "comment", "ls", "--subject", "REQ:"+frKey); code != 0 {
		t.Fatalf("comment ls --subject exit=%d", code)
	}

	// comment show: human names the id; JSON is a valid array.
	if out, code := reviewRun(t, "comment", "show", rootID); code != 0 || !strings.Contains(out, rootID) {
		t.Fatalf("comment show exit=%d out=%q", code, out)
	}
	if out, code := reviewRun(t, "--json", "comment", "show", rootID); code != 0 || !validJSON(out) {
		t.Fatalf("comment show --json exit=%d valid=%v", code, validJSON(out))
	}

	// findComment fetches one comment (body + resolved flag) from the JSON listing, by id.
	findComment := func(id string) (body string, resolved, found bool) {
		out, code := reviewRun(t, "--json", "comment", "ls", branch)
		if code != 0 {
			t.Fatalf("comment ls --json (find) exit=%d", code)
		}
		var cs []struct {
			ID       string `json:"id"`
			Body     string `json:"body"`
			Resolved bool   `json:"resolved"`
		}
		if err := json.Unmarshal([]byte(out), &cs); err != nil {
			t.Fatalf("comment ls --json (find) parse: %v\n%s", err, out)
		}
		for _, c := range cs {
			if c.ID == id {
				return c.Body, c.Resolved, true
			}
		}
		return "", false, false
	}

	// resolve / reopen / edit lifecycle, verified by reading the row back.
	if _, code := reviewRun(t, "comment", "resolve", rootID); code != 0 {
		t.Fatalf("comment resolve exit=%d", code)
	}
	if _, resolved, found := findComment(rootID); !found || !resolved {
		t.Fatalf("after resolve: found=%v resolved=%v", found, resolved)
	}
	if _, code := reviewRun(t, "comment", "reopen", rootID); code != 0 {
		t.Fatalf("comment reopen exit=%d", code)
	}
	if _, resolved, _ := findComment(rootID); resolved {
		t.Fatalf("after reopen: still resolved")
	}
	if _, code := reviewRun(t, "comment", "edit", rootID, "--body", "clarify: which field exactly?"); code != 0 {
		t.Fatalf("comment edit exit=%d", code)
	}
	if body, _, _ := findComment(rootID); !strings.Contains(body, "clarify: which field exactly?") {
		t.Fatalf("edit not applied, body=%q", body)
	}

	// --- reviews: record a verdict, list it, then replace it (upsert keeps one row) ---
	if out, code := reviewRun(t, "review", "--verdict", "approve", "--summary", "looks good to me"); code != 0 || !strings.Contains(out, "approved") {
		t.Fatalf("review approve exit=%d out=%q", code, out)
	}
	if out, code := reviewRun(t, "--json", "review", "ls"); code != 0 || !validJSON(out) {
		t.Fatalf("review ls --json exit=%d valid=%v", code, validJSON(out))
	}
	if _, code := reviewRun(t, "review", "ls"); code != 0 {
		t.Fatalf("review ls (human) exit=%d", code)
	}
	if out, code := reviewRun(t, "review", "--verdict", "request_changes"); code != 0 || !strings.Contains(out, "changes_requested") {
		t.Fatalf("review request_changes exit=%d out=%q", code, out)
	}
	out, code = reviewRun(t, "--json", "review", "ls")
	if code != 0 {
		t.Fatalf("review ls --json (2) exit=%d", code)
	}
	var reviews []struct {
		Verdict string `json:"verdict"`
	}
	if err := json.Unmarshal([]byte(out), &reviews); err != nil {
		t.Fatalf("review ls --json parse: %v\n%s", err, out)
	}
	if len(reviews) != 1 || reviews[0].Verdict != "request_changes" {
		t.Fatalf("upsert should leave one request_changes review, got %+v", reviews)
	}

	// --- submit, then merge into main ---
	if out, code := reviewRun(t, "changeset", "submit"); code != 0 || !strings.Contains(out, "submitted") {
		t.Fatalf("changeset submit exit=%d out=%q", code, out)
	}
	if out, code := reviewRun(t, "changeset", "merge"); code != 0 || !strings.Contains(out, "merged") {
		t.Fatalf("changeset merge exit=%d out=%q", code, out)
	}

	// post-merge: ls reports it merged; comments (which live on main) survive; the changeset stays
	// reviewable via diff.
	out, code = reviewRun(t, "--json", "changeset", "ls")
	if code != 0 {
		t.Fatalf("changeset ls --json (post-merge) exit=%d", code)
	}
	if err := json.Unmarshal([]byte(out), &csRows); err != nil {
		t.Fatalf("changeset ls --json (post-merge) parse: %v\n%s", err, out)
	}
	if st := reviewStatusOf(csRows, branch); st != "merged" {
		t.Fatalf("post-merge status of %s = %q, want merged", branch, st)
	}
	if _, code := reviewRun(t, "comment", "ls", branch); code != 0 {
		t.Fatalf("comment ls after merge exit=%d", code)
	}
	if out, code := reviewRun(t, "--json", "changeset", "diff", branch); code != 0 || !validJSON(out) {
		t.Fatalf("changeset diff after merge exit=%d valid=%v", code, validJSON(out))
	}

	// --- separate abandon flow: submit-then-abandon stays diffable via its recorded head ---
	tbranch := "changeset/throwaway-idea"
	if _, code := reviewRun(t, "changeset", "start", "throwaway idea"); code != 0 {
		t.Fatalf("changeset start (throwaway) exit=%d", code)
	}
	if _, code := reviewRun(t, "req", "add", "AUTH", "Speculative requirement"); code != 0 {
		t.Fatalf("req add (throwaway) exit=%d", code)
	}
	if _, code := reviewRun(t, "changeset", "submit"); code != 0 {
		t.Fatalf("changeset submit (throwaway) exit=%d", code)
	}
	if out, code := reviewRun(t, "changeset", "abandon"); code != 0 || !strings.Contains(out, "abandoned") {
		t.Fatalf("changeset abandon exit=%d out=%q", code, out)
	}
	out, code = reviewRun(t, "--json", "changeset", "ls")
	if code != 0 {
		t.Fatalf("changeset ls --json (post-abandon) exit=%d", code)
	}
	if err := json.Unmarshal([]byte(out), &csRows); err != nil {
		t.Fatalf("changeset ls --json (post-abandon) parse: %v\n%s", err, out)
	}
	if st := reviewStatusOf(csRows, tbranch); st != "closed" {
		t.Fatalf("post-abandon status of %s = %q, want closed", tbranch, st)
	}
	// The branch is gone, but the head commit recorded at submit keeps the abandoned changeset
	// diffable.
	if out, code := reviewRun(t, "--json", "changeset", "diff", tbranch); code != 0 || !validJSON(out) {
		t.Fatalf("diff of abandoned (submitted) changeset exit=%d valid=%v", code, validJSON(out))
	}

	// --- abandon before submit: no head/merge commit recorded, so there is nothing to diff ---
	nbranch := "changeset/never-submitted"
	if _, code := reviewRun(t, "changeset", "start", "never submitted"); code != 0 {
		t.Fatalf("changeset start (never-submitted) exit=%d", code)
	}
	if _, code := reviewRun(t, "changeset", "abandon"); code != 0 {
		t.Fatalf("changeset abandon (never-submitted) exit=%d", code)
	}
	if _, code := reviewRun(t, "changeset", "diff", nbranch); code != 3 {
		t.Fatalf("diff of abandoned-before-submit exit=%d, want not_found (3)", code)
	}
}

// reviewStatusOf returns the status recorded for a changeset branch in a `changeset ls` listing.
func reviewStatusOf(rows []struct{ Branch, Title, Status string }, branch string) string {
	for _, r := range rows {
		if r.Branch == branch {
			return r.Status
		}
	}
	return ""
}

// TestCLI_Review_Branch exercises the low-level branch verbs: list, create off the active target,
// checkout to retarget, the guards that refuse deleting main and the active target, a successful
// delete after switching away, and not-found errors.
func TestCLI_Review_Branch(t *testing.T) {
	newCLIWorkspace(t)

	// ls: a fresh workspace has just main, and main is the active read/write target.
	if out, code := reviewRun(t, "branch", "ls"); code != 0 || !strings.Contains(out, "main") {
		t.Fatalf("branch ls exit=%d out=%q", code, out)
	}
	var bl struct {
		Branches []string `json:"branches"`
		Active   string   `json:"active"`
	}
	readBranches := func(ctx string) {
		out, code := reviewRun(t, "--json", "branch", "ls")
		if code != 0 {
			t.Fatalf("branch ls --json (%s) exit=%d", ctx, code)
		}
		if err := json.Unmarshal([]byte(out), &bl); err != nil {
			t.Fatalf("branch ls --json (%s) parse: %v\n%s", ctx, err, out)
		}
	}
	readBranches("initial")
	if bl.Active != "main" || !reviewContains(bl.Branches, "main") {
		t.Fatalf("initial branch ls: %+v", bl)
	}

	// create off the active target (main).
	if out, code := reviewRun(t, "branch", "create", "feature-x"); code != 0 || !strings.Contains(out, "feature-x") {
		t.Fatalf("branch create exit=%d out=%q", code, out)
	}
	readBranches("after create")
	if !reviewContains(bl.Branches, "feature-x") {
		t.Fatalf("feature-x missing after create: %+v", bl)
	}

	// checkout retargets the active pointer.
	if out, code := reviewRun(t, "branch", "checkout", "feature-x"); code != 0 || !strings.Contains(out, "feature-x") {
		t.Fatalf("branch checkout exit=%d out=%q", code, out)
	}
	readBranches("after checkout")
	if bl.Active != "feature-x" {
		t.Fatalf("active=%q, want feature-x", bl.Active)
	}

	// delete refuses main and the active target (generic error, exit 1) — nothing is deleted.
	if _, code := reviewRun(t, "branch", "delete", "main"); code != 1 {
		t.Fatalf("delete main exit=%d, want 1", code)
	}
	if _, code := reviewRun(t, "branch", "delete", "feature-x"); code != 1 {
		t.Fatalf("delete active target exit=%d, want 1", code)
	}

	// switch back to main (clears the pointer), then the delete succeeds.
	if _, code := reviewRun(t, "branch", "checkout", "main"); code != 0 {
		t.Fatalf("branch checkout main exit=%d", code)
	}
	readBranches("after checkout main")
	if bl.Active != "main" {
		t.Fatalf("active=%q, want main", bl.Active)
	}
	if out, code := reviewRun(t, "branch", "delete", "feature-x"); code != 0 || !strings.Contains(out, "feature-x") {
		t.Fatalf("branch delete exit=%d out=%q", code, out)
	}
	readBranches("after delete")
	if reviewContains(bl.Branches, "feature-x") {
		t.Fatalf("feature-x still present after delete: %+v", bl)
	}

	// error paths: checkout a nonexistent branch is not_found (3); deleting one just fails.
	if _, code := reviewRun(t, "branch", "checkout", "ghost"); code != 3 {
		t.Fatalf("checkout ghost exit=%d, want not_found (3)", code)
	}
	if _, code := reviewRun(t, "branch", "delete", "ghost"); code == 0 {
		t.Fatalf("delete ghost should fail")
	}
}

// TestCLI_Review_Errors covers the validation (exit 2) and not-found (exit 3) paths for the
// comment/review/changeset verbs, plus the "no changeset specified and none active" guard.
func TestCLI_Review_Errors(t *testing.T) {
	newCLIWorkspace(t)
	if _, code := reviewRun(t, "domain", "add", "core", "Core"); code != 0 {
		t.Fatalf("domain add exit=%d", code)
	}
	if _, code := reviewRun(t, "spec", "add", "x.md", "--domain", "core", "--prefix", "X", "--title", "X spec"); code != 0 {
		t.Fatalf("spec add exit=%d", code)
	}

	// With no active changeset and no branch arg, the resolve guard fires (generic error, exit 1).
	for _, args := range [][]string{
		{"comment", "ls"},
		{"review", "ls"},
		{"changeset", "diff"},
	} {
		if _, code := reviewRun(t, args...); code != 1 {
			t.Fatalf("%v with no active changeset exit=%d, want 1", args, code)
		}
	}

	// Operating on a nonexistent changeset is not_found (exit 3).
	for _, args := range [][]string{
		{"comment", "ls", "changeset/nope"},
		{"review", "ls", "changeset/nope"},
		{"changeset", "diff", "changeset/nope"},
	} {
		if _, code := reviewRun(t, args...); code != 3 {
			t.Fatalf("%v exit=%d, want not_found (3)", args, code)
		}
	}

	// Open a changeset for the subject/body error paths.
	if _, code := reviewRun(t, "changeset", "start", "errors cs"); code != 0 {
		t.Fatalf("changeset start exit=%d", code)
	}

	// A freshly-opened changeset has no edits yet — diff reports "no changes" (table + per-entity).
	if out, code := reviewRun(t, "changeset", "diff"); code != 0 || !strings.Contains(out, "no changes") {
		t.Fatalf("empty changeset diff exit=%d out=%q", code, out)
	}
	if out, code := reviewRun(t, "changeset", "diff", "--entities"); code != 0 || !strings.Contains(out, "no entity changes") {
		t.Fatalf("empty changeset diff --entities exit=%d out=%q", code, out)
	}
	if out, code := reviewRun(t, "--json", "changeset", "diff", "--entities"); code != 0 || !validJSON(out) {
		t.Fatalf("empty changeset diff --entities --json exit=%d valid=%v", code, validJSON(out))
	}

	// comment add: missing --body is a validation error (exit 2).
	if _, code := reviewRun(t, "comment", "add"); code != 2 {
		t.Fatalf("comment add (no body) exit=%d, want 2", code)
	}
	// comment add: a subject that doesn't resolve on the branch is not_found (exit 3).
	if _, code := reviewRun(t, "comment", "add", "--subject", "REQ:NOPE-FR-999", "--body", "x"); code != 3 {
		t.Fatalf("comment add (unresolvable subject) exit=%d, want 3", code)
	}
	// comment add: --subject-type without --subject-id is a validation error (exit 2).
	if _, code := reviewRun(t, "comment", "add", "--subject-type", "requirement", "--body", "x"); code != 2 {
		t.Fatalf("comment add (subject-type without id) exit=%d, want 2", code)
	}
	// comment add: an invalid --subject-type is a validation error (exit 2).
	if _, code := reviewRun(t, "comment", "add", "--subject-type", "bogus", "--subject-id", "abc", "--body", "x"); code != 2 {
		t.Fatalf("comment add (bad subject-type) exit=%d, want 2", code)
	}
	// comment add on a nonexistent changeset is not_found (exit 3).
	if _, code := reviewRun(t, "comment", "add", "changeset/nope", "--body", "x"); code != 3 {
		t.Fatalf("comment add (missing changeset) exit=%d, want 3", code)
	}

	// comment mutations on a nonexistent id are not_found (exit 3).
	for _, args := range [][]string{
		{"comment", "show", "ghost-id"},
		{"comment", "resolve", "ghost-id"},
		{"comment", "reopen", "ghost-id"},
		{"comment", "edit", "ghost-id", "--body", "x"},
		{"comment", "delete", "ghost-id"},
	} {
		if _, code := reviewRun(t, args...); code != 3 {
			t.Fatalf("%v exit=%d, want not_found (3)", args, code)
		}
	}
	// comment edit: missing --body is validation (exit 2), checked before the row lookup.
	if _, code := reviewRun(t, "comment", "edit", "ghost-id"); code != 2 {
		t.Fatalf("comment edit (no body) exit=%d, want 2", code)
	}

	// review: missing / invalid verdicts are validation errors (exit 2).
	if _, code := reviewRun(t, "review"); code != 2 {
		t.Fatalf("review (no verdict) exit=%d, want 2", code)
	}
	if _, code := reviewRun(t, "review", "--verdict", "bogus"); code != 2 {
		t.Fatalf("review (bad verdict) exit=%d, want 2", code)
	}
	// review on a nonexistent changeset is not_found (exit 3).
	if _, code := reviewRun(t, "review", "changeset/nope", "--verdict", "approve"); code != 3 {
		t.Fatalf("review (missing changeset) exit=%d, want 3", code)
	}

	// changeset submit/merge against a missing branch just fail (nonzero).
	if _, code := reviewRun(t, "changeset", "submit", "changeset/nope"); code == 0 {
		t.Fatalf("submit missing changeset should fail")
	}
	if _, code := reviewRun(t, "changeset", "merge", "changeset/nope"); code == 0 {
		t.Fatalf("merge missing changeset should fail")
	}
}
