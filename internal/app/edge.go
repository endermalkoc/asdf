package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/endermalkoc/cusp/internal/refs"
	"github.com/endermalkoc/cusp/internal/store"
)

// acyclicEdgeKinds are the edge kinds whose graph must stay a DAG — a cycle in any of
// them is a contradiction (A refines / depends_on / supersedes / defers_to B, …, back to
// A). `references` and `relates` may be cyclic (mutual links are legitimate).
var acyclicEdgeKinds = map[string]bool{
	"refines":    true,
	"depends_on": true,
	"supersedes": true,
	"defers_to":  true,
}

// ResolveRef resolves an edge endpoint (or any reference argument) to its target. The
// reference is a `TYPE:key` — e.g. SPEC:ADDS, ENTITY:Student, REQ:ATT-FR-012,
// DOMAIN:finance, MILESTONE:M4, TERM:makeup — optionally wrapped in `[[ ]]`. A bare value
// with no `TYPE:` is treated as a requirement fr_key (back-compat with the FR-only edge).
// ok is false when the reference is malformed or names no existing entity — so a single
// Resolve both validates existence and yields the endpoint's type+id.
func ResolveRef(resolver *refs.Resolver, arg string) (refs.Target, bool) {
	s := strings.TrimSpace(arg)
	s = strings.TrimSuffix(strings.TrimPrefix(s, "[["), "]]")
	if !strings.Contains(s, ":") {
		s = "REQ:" + s
	}
	toks := refs.Scan("[[" + s + "]]")
	if len(toks) != 1 || !toks[0].Known() {
		return refs.Target{}, false
	}
	return resolver.Resolve(toks[0])
}

// CheckEdgeAcyclic rejects an edge that would break the acyclicity its kind requires: a
// self-loop, or a from→to that closes a cycle because `to` already reaches `from` over
// edges of the same kind. It is a no-op for cyclic-allowed kinds. fromLabel/toLabel are
// the user's endpoint strings, used only in the error message.
func CheckEdgeAcyclic(ctx context.Context, x store.Execer, fromType, fromID, kind, toType, toID, fromLabel, toLabel string) error {
	if !acyclicEdgeKinds[kind] {
		return nil
	}
	if fromType == toType && fromID == toID {
		return fmt.Errorf("a %s edge cannot point a node at itself (%s)", kind, fromLabel)
	}
	edges, err := store.ListEdgesOfKind(ctx, x, kind)
	if err != nil {
		return err
	}
	if reaches(edges, EdgeNode{toType, toID}, EdgeNode{fromType, fromID}) {
		return fmt.Errorf("a %s edge from %s to %s would create a cycle: %s already reaches %s via %s",
			kind, fromLabel, toLabel, toLabel, fromLabel, kind)
	}
	return nil
}

// EdgeNode identifies a graph node by its polymorphic (type, id).
type EdgeNode struct{ Type, ID string }

// reaches reports whether dst is reachable from src by following edges from→to (BFS over
// the given same-kind edge set).
func reaches(edges []store.EdgeEndpoint, src, dst EdgeNode) bool {
	adj := map[EdgeNode][]EdgeNode{}
	for _, e := range edges {
		f := EdgeNode{e.FromType, e.FromID}
		adj[f] = append(adj[f], EdgeNode{e.ToType, e.ToID})
	}
	seen := map[EdgeNode]bool{src: true}
	for queue := []EdgeNode{src}; len(queue) > 0; {
		n := queue[0]
		queue = queue[1:]
		if n == dst {
			return true
		}
		for _, m := range adj[n] {
			if !seen[m] {
				seen[m] = true
				queue = append(queue, m)
			}
		}
	}
	return false
}
