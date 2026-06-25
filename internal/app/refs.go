package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/endermalkoc/asdf/internal/refs"
	"github.com/endermalkoc/asdf/internal/store"
)

// LoadResolver builds a cross-reference resolver from the store's ref targets — the
// (type,key)→entity index that `[[TYPE:key]]` tokens resolve against. Build once per
// command; the read sits on whatever branch x is connected to.
func LoadResolver(ctx context.Context, x store.Execer) (*refs.Resolver, error) {
	rows, err := store.ListRefTargets(ctx, x)
	if err != nil {
		return nil, err
	}
	targets := make([]refs.Target, len(rows))
	for i, r := range rows {
		targets[i] = refs.Target{Type: r.Type, Key: r.Key, ID: r.ID, DocPath: r.DocPath, Anchor: r.Anchor}
	}
	return refs.NewResolver(targets), nil
}

// ResolvedRefs is the outcome of scanning text fields for inline `[[TYPE:key]]`
// tokens: the distinct resolved targets (to reconcile into entity_ref) and the
// tokens that did not resolve (to block an interactive write or report on import).
type ResolvedRefs struct {
	Targets  []refs.Target
	Dangling []refs.Token
}

// ScanRefs scans every field for `[[TYPE:key]]` tokens, resolving each against
// resolver. A self-reference (target == owner) is dropped; distinct targets are
// de-duplicated. ownerID may be empty when the owner row does not exist yet (e.g.
// validating a create before the id is minted).
func ScanRefs(resolver *refs.Resolver, ownerType, ownerID string, fields ...string) ResolvedRefs {
	var out ResolvedRefs
	seen := map[string]bool{}
	for _, f := range fields {
		for _, t := range refs.Scan(f) {
			tg, ok := resolver.Resolve(t)
			if !ok {
				out.Dangling = append(out.Dangling, t)
				continue
			}
			if tg.Type == ownerType && tg.ID == ownerID {
				continue // self-reference
			}
			k := tg.Type + "\x00" + tg.ID
			if seen[k] {
				continue
			}
			seen[k] = true
			out.Targets = append(out.Targets, tg)
		}
	}
	return out
}

// DanglingError formats dangling tokens as a blocking validation error (nil when
// there are none). Interactive writes fail with this unless --force is set.
func DanglingError(d []refs.Token) error {
	if len(d) == 0 {
		return nil
	}
	raws := make([]string, len(d))
	for i, t := range d {
		raws[i] = t.Raw
	}
	return fmt.Errorf("unresolved cross-reference(s): %s (use --force to write anyway)", strings.Join(raws, ", "))
}

// ReconcileRefs replaces an owner's entity_ref rows with the given resolved targets
// (delete-all-then-insert so a removed token drops its row) and marks the table
// dirty. Call inside a Mutate body, after the owner id is known.
func ReconcileRefs(ctx context.Context, w *Write, ownerType, ownerID string, targets []refs.Target) error {
	if err := store.DeleteEntityRefsByOwner(ctx, w.Tx, ownerType, ownerID); err != nil {
		return err
	}
	for _, tg := range targets {
		if _, err := store.UpsertEntityRef(ctx, w.Tx, ownerType, ownerID, tg.Type, tg.ID, "references"); err != nil {
			return err
		}
	}
	w.MarkDirty("entity_ref")
	return nil
}
