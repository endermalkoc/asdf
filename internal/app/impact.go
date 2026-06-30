package app

import (
	"context"
	"sort"

	"github.com/endermalkoc/cusp/internal/refs"
	"github.com/endermalkoc/cusp/internal/store"
)

// ImpactLink is one relationship touching the subject: the other endpoint (resolved to a
// "type:key" label) and how they relate — "ref" for an inline cross-reference, or the
// edge kind for a structured edge.
type ImpactLink struct {
	Endpoint string `json:"endpoint"`
	Via      string `json:"via"`
}

// ImpactReport is the relationships around a subject node. Inbound = things that point AT
// the subject (what's affected if it changes); Outbound = things the subject points at
// (what it relies on). Transitive (when requested) is the set of nodes that reach the
// subject by following edges in reverse — the blast radius.
type ImpactReport struct {
	Subject    string       `json:"subject"`
	Inbound    []ImpactLink `json:"inbound"`
	Outbound   []ImpactLink `json:"outbound"`
	Transitive []string     `json:"transitive,omitempty"`
}

// Impact gathers the references and edges touching subject, labeling every endpoint via a
// reverse (id → "type:key") index built from the store's ref targets. When transitive is
// set, it also computes the reverse-edge closure (everything that transitively depends on
// the subject through edges).
func Impact(ctx context.Context, x store.Execer, subject refs.Target, transitive bool) (ImpactReport, error) {
	lbl, err := LabelIndex(ctx, x)
	if err != nil {
		return ImpactReport{}, err
	}

	rep := ImpactReport{Subject: subject.Type + ":" + subject.Key}
	inSeen, outSeen := map[string]bool{}, map[string]bool{}
	addIn := func(endpoint, via string) {
		if k := endpoint + "|" + via; !inSeen[k] {
			inSeen[k] = true
			rep.Inbound = append(rep.Inbound, ImpactLink{Endpoint: endpoint, Via: via})
		}
	}
	addOut := func(endpoint, via string) {
		if k := endpoint + "|" + via; !outSeen[k] {
			outSeen[k] = true
			rep.Outbound = append(rep.Outbound, ImpactLink{Endpoint: endpoint, Via: via})
		}
	}

	refRows, err := store.ListEntityRefsFor(ctx, x, subject.Type, subject.ID)
	if err != nil {
		return ImpactReport{}, err
	}
	for _, r := range refRows {
		if r.TargetType == subject.Type && r.TargetID == subject.ID {
			addIn(lbl(r.OwnerType, r.OwnerID), "ref")
		}
		if r.OwnerType == subject.Type && r.OwnerID == subject.ID {
			addOut(lbl(r.TargetType, r.TargetID), "ref")
		}
	}

	edges, err := store.ListAllEdges(ctx, x)
	if err != nil {
		return ImpactReport{}, err
	}
	for _, e := range edges {
		if e.ToType == subject.Type && e.ToID == subject.ID {
			addIn(lbl(e.FromType, e.FromID), e.Kind)
		}
		if e.FromType == subject.Type && e.FromID == subject.ID {
			addOut(lbl(e.ToType, e.ToID), e.Kind)
		}
	}

	if transitive {
		radj := map[EdgeNode][]EdgeNode{}
		for _, e := range edges {
			to := EdgeNode{e.ToType, e.ToID}
			radj[to] = append(radj[to], EdgeNode{e.FromType, e.FromID})
		}
		self := EdgeNode{subject.Type, subject.ID}
		visited := map[EdgeNode]bool{self: true}
		for queue := []EdgeNode{self}; len(queue) > 0; {
			n := queue[0]
			queue = queue[1:]
			for _, m := range radj[n] {
				if !visited[m] {
					visited[m] = true
					rep.Transitive = append(rep.Transitive, lbl(m.Type, m.ID))
					queue = append(queue, m)
				}
			}
		}
		sort.Strings(rep.Transitive)
	}

	sortLinks(rep.Inbound)
	sortLinks(rep.Outbound)
	return rep, nil
}

// LabelIndex builds a reverse lookup from an entity's (type, id) to a readable "type:key"
// label, from the store's ref targets — for rendering graph endpoints (impact, edge ls).
// A (type, id) that is not a ref target (e.g. a user_story) falls back to "type:id". For
// specs, the prefix label wins over the path (it is listed first).
func LabelIndex(ctx context.Context, x store.Execer) (func(typ, id string) string, error) {
	targets, err := store.ListRefTargets(ctx, x)
	if err != nil {
		return nil, err
	}
	label := make(map[string]string, len(targets))
	for _, t := range targets {
		if k := t.Type + "\x00" + t.ID; label[k] == "" {
			label[k] = t.Type + ":" + t.Key
		}
	}
	return func(typ, id string) string {
		if l, ok := label[typ+"\x00"+id]; ok {
			return l
		}
		return typ + ":" + id
	}, nil
}

func sortLinks(ls []ImpactLink) {
	sort.Slice(ls, func(i, j int) bool {
		if ls[i].Endpoint != ls[j].Endpoint {
			return ls[i].Endpoint < ls[j].Endpoint
		}
		return ls[i].Via < ls[j].Via
	})
}
