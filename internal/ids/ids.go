// Package ids mints Cusp's primary keys.
//
// Two strategies, per docs/entities/identifiers.md:
//   - New() — a time-ordered ULID for authored rows (collision-free, offline-mintable).
//   - Rel(...) — a deterministic UUIDv5 over a relationship row's identity columns,
//     so the same edge/junction created on two branches converges on merge instead
//     of colliding (the deterministic-PK rule).
package ids

import (
	"strings"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
)

// relNamespace is the fixed UUIDv5 namespace for Cusp relationship ids. It is
// derived once from a stable URL and must never change: changing it would re-key
// every relationship row and reintroduce the cross-clone divergence Rel exists to
// prevent (cf. beads' depid/rowid, the technique this generalizes).
var relNamespace = uuid.NewSHA1(uuid.NameSpaceURL, []byte("https://github.com/endermalkoc/cusp#rel-id"))

// sep joins identity components. ASCII Unit Separator (0x1f) cannot occur in an
// id or an enum value, so no two distinct component sequences can collide.
const sep = "\x1f"

// New returns a fresh time-ordered ULID for an authored row's primary key.
func New() string {
	return ulid.Make().String()
}

// Rel returns the deterministic primary key for a pure-relationship row, derived
// from its identity columns (the columns of its UNIQUE key). The same parts in
// the same order always yield the same id — on every clone and in every process —
// which is what makes edges/junctions merge-safe across Dolt clones. Pass only the
// immutable identity columns, never mutable payload.
func Rel(parts ...string) string {
	return uuid.NewSHA1(relNamespace, []byte(strings.Join(parts, sep))).String()
}
