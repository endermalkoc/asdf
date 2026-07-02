package enums

import "testing"

func TestStatusForVerdict(t *testing.T) {
	cases := map[string]string{
		VerdictApprove:        ChangesetApproved,
		VerdictRequestChanges: ChangesetChangesRequested,
		VerdictDeny:           ChangesetDenied,
		"":                    "",
		"bogus":               "",
	}
	for verdict, want := range cases {
		if got := StatusForVerdict(verdict); got != want {
			t.Errorf("StatusForVerdict(%q) = %q, want %q", verdict, got, want)
		}
	}
}

func TestReviewVerdictSet(t *testing.T) {
	for _, v := range []string{"approve", "deny", "request_changes"} {
		if !Valid(ReviewVerdict, v) {
			t.Errorf("verdict %q should be valid", v)
		}
	}
	for _, v := range []string{"maybe", "approved", "reject", ""} {
		if v != "" && Valid(ReviewVerdict, v) {
			t.Errorf("verdict %q should be invalid", v)
		}
	}
	// Every allowed verdict must map to a changeset status (keeps the two in lockstep).
	for _, v := range ReviewVerdict {
		if StatusForVerdict(v) == "" {
			t.Errorf("verdict %q has no changeset status mapping", v)
		}
	}
}

func TestPlanningEnumSets(t *testing.T) {
	valid := map[*[]string][]string{
		&CapabilityLevel:    {"domain", "epic", "capability"},
		&DeliverableSize:    {"S", "M", "L", "XL"},
		&DeliverableStatus:  {"proposed", "specced", "wired", "built", "ship"},
		&DeliverableAIReady: {"yes", "no", "na"},
	}
	for set, vals := range valid {
		for _, v := range vals {
			if !Valid(*set, v) {
				t.Errorf("%q should be valid", v)
			}
		}
		if Valid(*set, "bogus") {
			t.Errorf("bogus should be invalid in %v", *set)
		}
	}
	// The deliverable-status constants must all be in the set.
	for _, c := range []string{DeliverableProposed, DeliverableSpecced, DeliverableWired, DeliverableBuilt, DeliverableShip} {
		if !Valid(DeliverableStatus, c) {
			t.Errorf("deliverable status const %q missing from set", c)
		}
	}
}

func TestTestingEnumSets(t *testing.T) {
	cases := map[*[]string][]string{
		&TestLayer:        {"unit", "e2e", "shared"},
		&TestType:         {"functional", "smoke", "other"},
		&TestSeverity:     {"trivial", "normal", "blocker"},
		&TestAutomation:   {"manual", "automated", "to_be_automated"},
		&TestCaseStatus:   {"draft", "active", "deprecated"},
		&TestRunStatus:    {"active", "complete", "aborted"},
		&TestResultStatus: {"passed", "failed", "skipped", "in_progress"},
	}
	for set, vals := range cases {
		for _, v := range vals {
			if !Valid(*set, v) {
				t.Errorf("%q should be valid", v)
			}
		}
		if Valid(*set, "nope") {
			t.Errorf("nope should be invalid in %v", *set)
		}
	}
}

func TestCommentSubjectTypeSet(t *testing.T) {
	for _, st := range []string{"requirement", "spec", "user_story", "test_case", "entity", "deliverable"} {
		if !Valid(CommentSubjectType, st) {
			t.Errorf("subject type %q should be valid", st)
		}
	}
	for _, st := range []string{"domain", "milestone", "glossary_term", "capability"} {
		if Valid(CommentSubjectType, st) {
			t.Errorf("subject type %q should NOT be a comment subject", st)
		}
	}
}
