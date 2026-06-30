package app

import (
	"testing"

	"github.com/endermalkoc/cusp/internal/store"
)

func n(id string) EdgeNode { return EdgeNode{Type: "requirement", ID: id} }

func TestReaches(t *testing.T) {
	// a → b → c, plus a cross-type d(spec) → e(entity).
	edges := []store.EdgeEndpoint{
		{FromType: "requirement", FromID: "a", ToType: "requirement", ToID: "b"},
		{FromType: "requirement", FromID: "b", ToType: "requirement", ToID: "c"},
		{FromType: "spec", FromID: "d", ToType: "entity", ToID: "e"},
	}
	cases := []struct {
		name     string
		src, dst EdgeNode
		want     bool
	}{
		{"direct", n("a"), n("b"), true},
		{"transitive", n("a"), n("c"), true},
		{"reverse not reachable", n("c"), n("a"), false},
		{"unrelated", n("a"), n("z"), false},
		{"cross-type direct", EdgeNode{"spec", "d"}, EdgeNode{"entity", "e"}, true},
		{"type matters", EdgeNode{"requirement", "d"}, EdgeNode{"entity", "e"}, false},
	}
	for _, c := range cases {
		if got := reaches(edges, c.src, c.dst); got != c.want {
			t.Errorf("%s: reaches(%v,%v)=%v want %v", c.name, c.src, c.dst, got, c.want)
		}
	}
}

func TestEdgeKindAcyclicSet(t *testing.T) {
	// The acyclic kinds reject cycles; references/relates permit them.
	for _, k := range []string{"refines", "depends_on", "supersedes", "defers_to"} {
		if !acyclicEdgeKinds[k] {
			t.Errorf("%q should be an acyclic edge kind", k)
		}
	}
	for _, k := range []string{"references", "relates"} {
		if acyclicEdgeKinds[k] {
			t.Errorf("%q should permit cycles", k)
		}
	}
}
