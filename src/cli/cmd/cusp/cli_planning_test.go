package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// In-process CLI tests for the planning command group: milestone, capability, deliverable, and
// view (add/ls/show/edit/delete plus the capability/deliverable link+unlink subcommands). Each
// test starts its own Dolt workspace via newCLIWorkspace and then SWEEPS many related commands on
// that one server to keep server-start cost (~6s each) amortized. Shared setup + runCLI live in
// cli_harness_test.go; these tests must not redefine those helpers.
//
// Ordering note: several cobra flags (and their pflag "Changed" state) persist across in-process
// Execute calls because the command tree is a package global and pflag never clears Changed. So
// each edit/add command is exercised in exactly ONE test function, the missing-required-flag and
// "nothing to edit" guards are asserted on the FIRST touch of a command, and error paths that set
// invalid values on shared flag vars run AFTER the happy paths. viewAddCmd is used both in the
// view sweep (where its required-flag guard is asserted) and in the deliverable sweep (as a link
// target), so the view sweep is declared before the deliverable sweep to keep the guard on a
// pristine command.

// planningReset zeroes the planning command-local flag vars so a value set by an earlier Execute
// (the vars are package globals) cannot bleed into a later command that reads them directly.
func planningReset() {
	msDescription, msStatus, milestoneName = "", "", ""
	msSequence = 0
	capLevel, capDomain, capParent, capMilestone, capDeliverable, capTitle = "", "", "", "", "", ""
	delivSize, delivStatus, delivAIReady, delivMilestone, delivTitle, delivView, delivBlockedBy = "", "", "", "", "", "", ""
	viewDomain, viewRoute, viewSpec, viewTitle = "", "", "", ""
}

// planningOK runs a command expecting exit 0 and returns its stdout.
func planningOK(t *testing.T, args ...string) string {
	t.Helper()
	out, code := runCLI(t, args...)
	if code != 0 {
		t.Fatalf("cusp %v: expected exit 0, got %d\n%s", args, code, out)
	}
	return out
}

// planningExit runs a command expecting a specific exit code and returns its stdout.
func planningExit(t *testing.T, want int, args ...string) string {
	t.Helper()
	out, code := runCLI(t, args...)
	if code != want {
		t.Fatalf("cusp %v: expected exit %d, got %d\n%s", args, want, code, out)
	}
	return out
}

// planningContains fails unless sub is present in out.
func planningContains(t *testing.T, out, sub string) {
	t.Helper()
	if !strings.Contains(out, sub) {
		t.Fatalf("expected output to contain %q, got:\n%s", sub, out)
	}
}

// planningIDFromJSON extracts the "id" field emitted by an `add --json` command.
func planningIDFromJSON(t *testing.T, out string) string {
	t.Helper()
	var v struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("parse id from json: %v\n%s", err, out)
	}
	if v.ID == "" {
		t.Fatalf("no id in json output: %s", out)
	}
	return v.ID
}

// planningJSONLen unmarshals a JSON array and returns its length.
func planningJSONLen(t *testing.T, out string) int {
	t.Helper()
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Fatalf("parse json array: %v\n%s", err, out)
	}
	return len(arr)
}

// TestCLI_Planning_MilestoneSweep exercises milestone add/ls/show/edit/delete (keyed by slug),
// the alias `ms`, and the validation / not-found / nothing-to-edit error paths.
func TestCLI_Planning_MilestoneSweep(t *testing.T) {
	newCLIWorkspace(t)
	planningReset()

	// add: a plain one first (so add's Changed("sequence") is still false -> null sequence),
	// then one carrying every optional flag.
	planningOK(t, "milestone", "add", "M9", "Milestone Nine")
	planningOK(t, "ms", "add", "M8", "Milestone Eight", "--description", "eighth", "--status", "in_progress", "--sequence", "8")

	// ls (human + json).
	lsOut := planningOK(t, "milestone", "ls")
	planningContains(t, lsOut, "M8")
	planningContains(t, lsOut, "M9")
	lsJSON := planningOK(t, "milestone", "ls", "--json")
	if !validJSON(lsJSON) || planningJSONLen(t, lsJSON) < 2 {
		t.Fatalf("milestone ls --json bad: %s", lsJSON)
	}

	// show (human + json).
	showOut := planningOK(t, "milestone", "show", "M8")
	planningContains(t, showOut, "Milestone Eight")
	planningContains(t, showOut, "sequence:")
	planningContains(t, showOut, "in_progress")
	showJSON := planningOK(t, "milestone", "show", "M8", "--json")
	if !validJSON(showJSON) {
		t.Fatalf("milestone show --json invalid: %s", showJSON)
	}

	// nothing-to-edit guard — must be the first touch of milestoneEditCmd (NFlag == 0 -> exit 1).
	planningExit(t, 1, "milestone", "edit", "M8")

	// real edit: change every editable field, then confirm via show.
	planningOK(t, "milestone", "edit", "M8", "--name", "Renamed Eight", "--status", "complete", "--sequence", "10", "--description", "updated")
	edited := planningOK(t, "milestone", "show", "M8")
	planningContains(t, edited, "Renamed Eight")
	planningContains(t, edited, "complete")

	// error paths (invalid enum values pollute the shared vars, so they run last).
	planningExit(t, 3, "milestone", "show", "nope")                       // not_found
	planningExit(t, 3, "milestone", "edit", "nope", "--name", "x")        // not_found (guard passes: name Changed)
	planningExit(t, 3, "milestone", "delete", "nope")                     // not_found
	planningExit(t, 2, "milestone", "edit", "M8", "--status", "bogus")    // validation
	planningExit(t, 2, "milestone", "add", "MX", "Bad", "--status", "no") // validation

	// delete a real milestone and confirm it is gone.
	planningOK(t, "milestone", "delete", "M9")
	planningExit(t, 3, "milestone", "show", "M9")
}

// TestCLI_Planning_CapabilitySweep exercises capability add/ls/show/edit/delete and link/unlink to
// milestones and deliverables (capabilities are keyed by ULID id, captured from `add --json`).
func TestCLI_Planning_CapabilitySweep(t *testing.T) {
	newCLIWorkspace(t)
	planningReset()

	// prerequisites: domains, a milestone and a deliverable to link against.
	planningOK(t, "domain", "add", "core", "Core")
	planningOK(t, "domain", "add", "aux", "Aux")
	planningOK(t, "milestone", "add", "M1", "Milestone One")

	// missing required --domain — assert on the FIRST touch of capabilityAddCmd (Changed==false).
	planningExit(t, 1, "capability", "add", "NoDomain")

	delivOut := planningOK(t, "deliverable", "add", "Deliv Link Target", "--json")
	delivID := planningIDFromJSON(t, delivOut)

	// happy adds — capture the ULID ids.
	parentOut := planningOK(t, "capability", "add", "Parent Cap", "--domain", "core", "--level", "epic", "--json")
	parentID := planningIDFromJSON(t, parentOut)
	childOut := planningOK(t, "cap", "add", "Child Cap", "--domain", "core", "--level", "capability", "--parent", parentID, "--json")
	childID := planningIDFromJSON(t, childOut)

	// ls (human + json).
	lsOut := planningOK(t, "capability", "ls")
	planningContains(t, lsOut, parentID)
	lsJSON := planningOK(t, "capability", "ls", "--json")
	if !validJSON(lsJSON) || planningJSONLen(t, lsJSON) < 2 {
		t.Fatalf("capability ls --json bad: %s", lsJSON)
	}

	// show the child (parent still set here, before the parent-clearing edit below).
	showOut := planningOK(t, "capability", "show", childID)
	planningContains(t, showOut, "Child Cap")
	planningContains(t, showOut, "parent:")
	planningContains(t, showOut, parentID)
	if s := planningOK(t, "capability", "show", childID, "--json"); !validJSON(s) {
		t.Fatalf("capability show --json invalid: %s", s)
	}

	// nothing-to-edit guard — first touch of capabilityEditCmd.
	planningExit(t, 1, "capability", "edit", parentID)

	// real edits: retitle/relevel/redomain, then clear the parent link.
	planningOK(t, "capability", "edit", childID, "--title", "Child Renamed", "--level", "domain", "--domain", "aux")
	planningOK(t, "capability", "edit", childID, "--parent", "")
	planningContains(t, planningOK(t, "capability", "show", childID), "Child Renamed")

	// link/unlink to a milestone and a deliverable. Both target flags are always passed explicitly
	// (one empty) so the shared vars are deterministic regardless of a prior call.
	planningOK(t, "capability", "link", parentID, "--milestone", "M1", "--deliverable", "")
	planningOK(t, "capability", "link", parentID, "--milestone", "", "--deliverable", delivID)
	planningOK(t, "capability", "unlink", parentID, "--milestone", "M1", "--deliverable", "")
	planningOK(t, "capability", "unlink", parentID, "--milestone", "", "--deliverable", delivID)

	// error paths.
	planningExit(t, 3, "capability", "add", "X", "--domain", "ghost")                                                // unknown domain
	planningExit(t, 2, "capability", "add", "X", "--domain", "core", "--level", "bogus")                             // invalid level
	planningExit(t, 3, "capability", "add", "X", "--domain", "core", "--level", "capability", "--parent", "ghostid") // parent not_found
	planningExit(t, 3, "capability", "show", "ghost")                                                                // not_found
	planningExit(t, 3, "capability", "edit", "ghost", "--title", "X")                                                // not_found
	planningExit(t, 2, "capability", "link", parentID, "--milestone", "", "--deliverable", "")                       // neither target
	planningExit(t, 3, "capability", "link", "ghost", "--milestone", "M1", "--deliverable", "")                      // capability not_found
	planningExit(t, 3, "capability", "link", parentID, "--milestone", "ZZZ", "--deliverable", "")                    // milestone not_found
	planningExit(t, 3, "capability", "link", parentID, "--milestone", "", "--deliverable", "ghostid")                // deliverable not_found

	// delete (missing then real).
	planningExit(t, 3, "capability", "delete", "ghost")
	planningOK(t, "capability", "delete", childID)
	planningOK(t, "capability", "delete", parentID)
}

// TestCLI_Planning_ViewSweep exercises view add/ls/show/edit/delete (keyed by ULID id), including
// the optional backing-spec soft FK and the required-domain / not-found error paths. Declared
// before the deliverable sweep so viewAddCmd's required-flag guard is asserted on a pristine cmd.
func TestCLI_Planning_ViewSweep(t *testing.T) {
	newCLIWorkspace(t)
	planningReset()

	// prerequisites: domains and a spec to back a view.
	planningOK(t, "domain", "add", "core", "Core")
	planningOK(t, "domain", "add", "aux", "Aux")
	planningOK(t, "spec", "add", "mainview.md", "--domain", "core", "--title", "Main View")

	// missing required --domain — first touch of viewAddCmd.
	planningExit(t, 1, "view", "add", "NoDomain")

	// happy adds (one with a route + backing spec, one bare), capturing ULID ids.
	dashOut := planningOK(t, "view", "add", "Dashboard", "--domain", "core", "--route", "/dash", "--spec", "mainview", "--json")
	viewID := planningIDFromJSON(t, dashOut)
	setOut := planningOK(t, "view", "add", "Settings", "--domain", "core", "--json")
	viewID2 := planningIDFromJSON(t, setOut)

	// ls (human + json).
	lsOut := planningOK(t, "view", "ls")
	planningContains(t, lsOut, viewID)
	lsJSON := planningOK(t, "view", "ls", "--json")
	if !validJSON(lsJSON) || planningJSONLen(t, lsJSON) < 2 {
		t.Fatalf("view ls --json bad: %s", lsJSON)
	}

	// show (human + json).
	showOut := planningOK(t, "view", "show", viewID)
	planningContains(t, showOut, "Dashboard")
	planningContains(t, showOut, "route:")
	planningContains(t, showOut, "/dash")
	if s := planningOK(t, "view", "show", viewID, "--json"); !validJSON(s) {
		t.Fatalf("view show --json invalid: %s", s)
	}

	// nothing-to-edit guard — first touch of viewEditCmd.
	planningExit(t, 1, "view", "edit", viewID)

	// real edit: retitle/reroute/redomain/respec, then confirm.
	planningOK(t, "view", "edit", viewID, "--title", "Dash Two", "--route", "/dash2", "--domain", "aux", "--spec", "mainview")
	planningContains(t, planningOK(t, "view", "show", viewID), "Dash Two")

	// error paths.
	planningExit(t, 3, "view", "add", "X", "--domain", "ghost")                          // unknown domain
	planningExit(t, 3, "view", "add", "X", "--domain", "core", "--spec", "no-such-spec") // no unique spec
	planningExit(t, 3, "view", "show", "ghost")                                          // not_found
	planningExit(t, 3, "view", "edit", "ghost", "--title", "X")                          // not_found

	// delete (missing then real).
	planningExit(t, 3, "view", "delete", "ghost")
	planningOK(t, "view", "delete", viewID2)
	planningOK(t, "view", "delete", viewID)
}

// TestCLI_Planning_DeliverableSweep exercises deliverable add/ls/show/edit/delete (keyed by ULID
// id) plus link/unlink to a view and a blocking deliverable, and the enum/not-found error paths.
func TestCLI_Planning_DeliverableSweep(t *testing.T) {
	newCLIWorkspace(t)
	planningReset()

	// prerequisites: a domain, two milestones, and a view to link against.
	planningOK(t, "domain", "add", "core", "Core")
	planningOK(t, "milestone", "add", "M1", "Milestone One")
	planningOK(t, "milestone", "add", "M2", "Milestone Two")
	viewOut := planningOK(t, "view", "add", "Link Target", "--domain", "core", "--json")
	viewID := planningIDFromJSON(t, viewOut)

	// happy adds — one fully specified, one bare — capturing ids.
	aOut := planningOK(t, "deliverable", "add", "Deliv A", "--size", "M", "--status", "proposed", "--ai-ready", "yes", "--milestone", "M1", "--json")
	delivA := planningIDFromJSON(t, aOut)
	bOut := planningOK(t, "deliv", "add", "Deliv B", "--json")
	delivB := planningIDFromJSON(t, bOut)

	// ls (human + json).
	lsOut := planningOK(t, "deliverable", "ls")
	planningContains(t, lsOut, delivA)
	lsJSON := planningOK(t, "deliverable", "ls", "--json")
	if !validJSON(lsJSON) || planningJSONLen(t, lsJSON) < 2 {
		t.Fatalf("deliverable ls --json bad: %s", lsJSON)
	}

	// show (human + json).
	showOut := planningOK(t, "deliverable", "show", delivA)
	planningContains(t, showOut, "Deliv A")
	planningContains(t, showOut, "M1")
	if s := planningOK(t, "deliverable", "show", delivA, "--json"); !validJSON(s) {
		t.Fatalf("deliverable show --json invalid: %s", s)
	}

	// nothing-to-edit guard — first touch of deliverableEditCmd.
	planningExit(t, 1, "deliverable", "edit", delivA)

	// real edit: change every field, then confirm.
	planningOK(t, "deliverable", "edit", delivA, "--title", "Deliv A2", "--size", "L", "--status", "built", "--ai-ready", "no", "--milestone", "M2")
	edited := planningOK(t, "deliverable", "show", delivA)
	planningContains(t, edited, "Deliv A2")
	planningContains(t, edited, "built")

	// link/unlink to a view and to a blocking deliverable (both flags always explicit).
	planningOK(t, "deliverable", "link", delivA, "--view", viewID, "--blocked-by", "")
	planningOK(t, "deliverable", "link", delivA, "--view", "", "--blocked-by", delivB)
	planningOK(t, "deliverable", "unlink", delivA, "--view", viewID, "--blocked-by", "")
	planningOK(t, "deliverable", "unlink", delivA, "--view", "", "--blocked-by", delivB)

	// error paths — full valid enum sets except the one field under test (validation reads the
	// shared vars, so keep the others valid).
	planningExit(t, 2, "deliverable", "add", "X", "--size", "XXL", "--status", "proposed", "--ai-ready", "yes")                     // invalid size
	planningExit(t, 2, "deliverable", "add", "X", "--size", "M", "--status", "nope", "--ai-ready", "yes")                           // invalid status
	planningExit(t, 2, "deliverable", "add", "X", "--size", "M", "--status", "proposed", "--ai-ready", "maybe")                     // invalid ai-ready
	planningExit(t, 3, "deliverable", "add", "X", "--size", "M", "--status", "proposed", "--ai-ready", "yes", "--milestone", "ZZZ") // milestone not_found
	planningExit(t, 3, "deliverable", "show", "ghost")                                                                              // not_found
	planningExit(t, 3, "deliverable", "edit", "ghost", "--title", "X")                                                              // not_found
	planningExit(t, 2, "deliverable", "edit", delivA, "--size", "XXL")                                                              // invalid enum on edit
	planningExit(t, 2, "deliverable", "link", delivA, "--view", "", "--blocked-by", "")                                             // neither target
	planningExit(t, 3, "deliverable", "link", "ghost", "--view", viewID, "--blocked-by", "")                                        // deliverable not_found
	planningExit(t, 3, "deliverable", "link", delivA, "--view", "ghostid", "--blocked-by", "")                                      // view not_found
	planningExit(t, 3, "deliverable", "link", delivA, "--view", "", "--blocked-by", "ghostid")                                      // blocker not_found

	// delete (missing then real).
	planningExit(t, 3, "deliverable", "delete", "ghost")
	planningOK(t, "deliverable", "delete", delivB)
	planningOK(t, "deliverable", "delete", delivA)
}
