package refs

import "testing"

func TestScan(t *testing.T) {
	toks := Scan("see [[REQ:ATT-FR-012]] and [[ENTITY:Student|the learner]], also [[TERM:makeup]] and [[BOGUS:x]]")
	if len(toks) != 4 {
		t.Fatalf("want 4 tokens, got %d", len(toks))
	}
	if toks[0].Type != TypeRequirement || toks[0].Key != "ATT-FR-012" || toks[0].Label() != "ATT-FR-012" {
		t.Errorf("req token wrong: %+v", toks[0])
	}
	if toks[1].Type != TypeEntity || toks[1].Display != "the learner" || toks[1].Label() != "the learner" {
		t.Errorf("entity token wrong: %+v", toks[1])
	}
	if toks[2].Type != TypeTerm || !toks[2].Known() { // TERM now resolves to a glossary_term
		t.Errorf("TERM should be a Known glossary_term: %+v", toks[2])
	}
	if toks[3].Type != "" || toks[3].Known() { // unknown tag
		t.Errorf("BOGUS should be unknown: %+v", toks[3])
	}
}

func newTestResolver() *Resolver {
	return NewResolver([]Target{
		{Type: TypeRequirement, Key: "ATT-FR-012", ID: "r1", DocPath: "scheduling/events/take-attendance.md", Anchor: "att-fr-012"},
		{Type: TypeEntity, Key: "Student", ID: "e1", DocPath: "entities/student.md"},
		{Type: TypeSpec, Key: "PRC", ID: "s1", DocPath: "finance/pricing.md"},
		{Type: TypeMilestone, Key: "M4", ID: "m1"}, // no page → label-only
	})
}

func TestRenderInline(t *testing.T) {
	r := newTestResolver()
	cases := []struct {
		name, owner, in, want string
		dangling              int
	}{
		{"cross-dir req", "finance/family-list.md",
			"per [[REQ:ATT-FR-012]] today",
			"per [ATT-FR-012](../scheduling/events/take-attendance.md#att-fr-012) today", 0},
		{"same-file req → in-page anchor", "scheduling/events/take-attendance.md",
			"refines [[REQ:ATT-FR-012]]",
			"refines [ATT-FR-012](#att-fr-012)", 0},
		{"same-dir spec", "finance/family-list.md",
			"see [[SPEC:PRC|pricing]]",
			"see [pricing](pricing.md)", 0},
		{"entity case-insensitive", "scheduling/events/take-attendance.md",
			"the [[ENTITY:student]] record",
			"the [student](../../entities/student.md) record", 0},
		{"milestone label-only", "finance/pricing.md",
			"ships in [[MILESTONE:M4]]",
			"ships in M4", 0},
		{"dangling left verbatim", "finance/pricing.md",
			"bad [[REQ:NOPE-FR-999]] ref",
			"bad [[REQ:NOPE-FR-999]] ref", 1},
	}
	for _, c := range cases {
		got, dangling := RenderInline(c.in, c.owner, r)
		if got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
		if len(dangling) != c.dangling {
			t.Errorf("%s: got %d dangling want %d", c.name, len(dangling), c.dangling)
		}
	}
}
