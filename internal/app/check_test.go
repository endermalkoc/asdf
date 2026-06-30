package app

import (
	"strings"
	"testing"

	"github.com/endermalkoc/cusp/internal/store"
)

func TestFindCycle(t *testing.T) {
	chain := []store.EdgeEndpoint{
		{FromType: "requirement", FromID: "a", ToType: "requirement", ToID: "b"},
		{FromType: "requirement", FromID: "b", ToType: "requirement", ToID: "c"},
	}
	if c := findCycle(chain); c != "" {
		t.Errorf("acyclic chain reported a cycle: %q", c)
	}

	cyclic := append(append([]store.EdgeEndpoint{}, chain...),
		store.EdgeEndpoint{FromType: "requirement", FromID: "c", ToType: "requirement", ToID: "a"})
	c := findCycle(cyclic)
	if c == "" {
		t.Fatal("expected a cycle, got none")
	}
	// The reported path closes on itself (first node repeated at the end).
	parts := strings.Split(c, " -> ")
	if parts[0] != parts[len(parts)-1] {
		t.Errorf("cycle path should close on itself: %q", c)
	}

	if findCycle([]store.EdgeEndpoint{{FromType: "spec", FromID: "x", ToType: "spec", ToID: "x"}}) == "" {
		t.Error("self-loop should be a cycle")
	}

	// Cross-type chain that does not loop.
	if c := findCycle([]store.EdgeEndpoint{{FromType: "spec", FromID: "d", ToType: "entity", ToID: "e"}}); c != "" {
		t.Errorf("non-looping cross-type edge reported a cycle: %q", c)
	}
}
