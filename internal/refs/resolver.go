package refs

import "strings"

// Target is a resolvable cross-reference destination: the entity's type+id (for the
// queryable entity_ref row) plus, when it has a generated page, the page path and an
// in-page anchor (for rendering a Markdown link). DocPath=="" means no page yet
// (domain/milestone) → the token renders as plain label text but still resolves.
type Target struct {
	Type    string
	Key     string
	ID      string
	DocPath string
	Anchor  string
	Label   string // human display title, used to render clean link text (no raw paths/.md)
}

// Resolver maps (type, key) → Target. Build once per generate/import run from the
// store's RefTargets; lookups are case-insensitive on the key.
type Resolver struct {
	byTypeKey map[string]Target
}

func indexKey(typ, k string) string { return typ + "\x00" + strings.ToLower(k) }

// NewResolver indexes the given targets. Later entries win on a duplicate key.
func NewResolver(targets []Target) *Resolver {
	m := make(map[string]Target, len(targets))
	for _, t := range targets {
		m[indexKey(t.Type, t.Key)] = t
	}
	return &Resolver{byTypeKey: m}
}

// Resolve maps a token to its Target. ok=false for unknown tags, TERM, or danglers.
func (r *Resolver) Resolve(t Token) (Target, bool) {
	if !t.Known() {
		return Target{}, false
	}
	tg, ok := r.byTypeKey[indexKey(t.Type, t.Key)]
	return tg, ok
}
