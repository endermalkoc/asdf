package app

import (
	"context"
	"sort"
	"strings"

	"github.com/endermalkoc/cusp/internal/refs"
	"github.com/endermalkoc/cusp/internal/store"
)

// CheckFinding is one integrity problem `check` reports.
type CheckFinding struct {
	Kind     string `json:"kind"`     // "dangling-ref" | "edge-cycle"
	Location string `json:"location"` // where it lives (entity label, or edge kind)
	Detail   string `json:"detail"`   // the offending token / cycle path
}

// Check runs the whole-graph integrity scan against x (a read handle, so it honors the
// branch x is on). It (1) scans every stored prose field for inline [[TYPE:key]] tokens
// that don't resolve to an existing entity, and (2) audits each acyclic edge kind for a
// cycle. Findings are returned sorted by kind+location for stable output.
func Check(ctx context.Context, x store.Execer) ([]CheckFinding, error) {
	resolver, err := LoadResolver(ctx, x)
	if err != nil {
		return nil, err
	}

	var out []CheckFinding

	// 1. Dangling inline references.
	fields, err := store.ListProseFields(ctx, x)
	if err != nil {
		return nil, err
	}
	for _, f := range fields {
		for _, t := range refs.Scan(f.Text) {
			if !t.Known() {
				continue // unknown tag (e.g. [[BOGUS:x]]) — not a recognized ref type
			}
			if _, ok := resolver.Resolve(t); !ok {
				out = append(out, CheckFinding{Kind: "dangling-ref", Location: f.Location, Detail: t.Raw})
			}
		}
	}

	// 2. Edge cycles (defensive — `edge add` blocks new cycles, this audits the graph).
	for kind := range acyclicEdgeKinds {
		edges, err := store.ListEdgesOfKind(ctx, x, kind)
		if err != nil {
			return nil, err
		}
		if cyc := findCycle(edges); cyc != "" {
			out = append(out, CheckFinding{Kind: "edge-cycle", Location: kind, Detail: cyc})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Location != out[j].Location {
			return out[i].Location < out[j].Location
		}
		return out[i].Detail < out[j].Detail
	})
	return out, nil
}

// findCycle returns a "a -> b -> … -> a" description of a cycle in the directed edge set,
// or "" if it is acyclic (DFS with a gray/black coloring; reports the first cycle found).
func findCycle(edges []store.EdgeEndpoint) string {
	adj := map[EdgeNode][]EdgeNode{}
	nodes := map[EdgeNode]bool{}
	for _, e := range edges {
		f, t := EdgeNode{e.FromType, e.FromID}, EdgeNode{e.ToType, e.ToID}
		adj[f] = append(adj[f], t)
		nodes[f], nodes[t] = true, true
	}
	const white, gray, black = 0, 1, 2
	color := map[EdgeNode]int{}
	var stack, cycle []EdgeNode
	var dfs func(n EdgeNode) bool
	dfs = func(n EdgeNode) bool {
		color[n] = gray
		stack = append(stack, n)
		for _, m := range adj[n] {
			switch color[m] {
			case gray:
				for i, s := range stack {
					if s == m {
						cycle = append(append([]EdgeNode{}, stack[i:]...), m)
						return true
					}
				}
			case white:
				if dfs(m) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[n] = black
		return false
	}
	// Visit in a stable order so the reported cycle is deterministic.
	ordered := make([]EdgeNode, 0, len(nodes))
	for n := range nodes {
		ordered = append(ordered, n)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Type != ordered[j].Type {
			return ordered[i].Type < ordered[j].Type
		}
		return ordered[i].ID < ordered[j].ID
	})
	for _, n := range ordered {
		if color[n] == white && dfs(n) {
			parts := make([]string, len(cycle))
			for i, s := range cycle {
				parts[i] = s.Type + ":" + s.ID
			}
			return strings.Join(parts, " -> ")
		}
	}
	return ""
}
