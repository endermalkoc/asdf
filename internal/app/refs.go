package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/endermalkoc/cusp/internal/refs"
	"github.com/endermalkoc/cusp/internal/store"
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
// validating a create before the id is minted). Thin wrapper over refs.ScanResolved —
// the same scan the importer uses against its staging-graph resolver.
func ScanRefs(resolver *refs.Resolver, ownerType, ownerID string, fields ...string) ResolvedRefs {
	targets, dangling := refs.ScanResolved(resolver, ownerType, ownerID, fields...)
	return ResolvedRefs{Targets: targets, Dangling: dangling}
}

// IngestRefs is the text-ingestion entry point for the write commands: it rewrites each
// field's raw inline references into canonical `[[TYPE:key]]` tokens (refs.Canonicalize)
// and resolves them. It returns the rewritten fields, in the same order, to be STORED —
// so a value created from the CLI carries the same canonical links as an imported one —
// and the resolved refs (targets to reconcile, danglers to block or report). The
// importer applies the same two steps (refs.Canonicalize + refs.ScanResolved) against
// its graph resolver, so both paths generate and validate links identically.
func IngestRefs(resolver *refs.Resolver, ownerType, ownerID string, fields ...string) ([]string, ResolvedRefs) {
	rewritten := make([]string, len(fields))
	for i, f := range fields {
		rewritten[i] = refs.Canonicalize(f, resolver, ownerType, ownerID)
	}
	return rewritten, ScanRefs(resolver, ownerType, ownerID, rewritten...)
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
	return coded(ExitDangling, "dangling_ref",
		fmt.Errorf("unresolved cross-reference(s): %s (use --force to write anyway)", strings.Join(raws, ", ")))
}

// ReconcileRefs replaces an owner's entity_ref rows with the given resolved targets
// (delete-all-then-insert so a removed token drops its row) and marks the table
// dirty. Call inside a Mutate body, after the owner id is known.
func ReconcileRefs(ctx context.Context, w *Write, ownerType, ownerID string, targets []refs.Target) error {
	if err := store.DeleteEntityRefsByOwner(ctx, w.Tx, ownerType, ownerID); err != nil {
		return err
	}
	for _, tg := range targets {
		if _, err := store.UpsertEntityRef(ctx, w.Tx, ownerType, ownerID, tg.Type, tg.ID); err != nil {
			return err
		}
	}
	w.MarkDirty("req_entity_ref")
	return nil
}
