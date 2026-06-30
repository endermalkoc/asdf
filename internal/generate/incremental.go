package generate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/endermalkoc/cusp/internal/store"
)

// Target is one configured output: a format rendered into a directory. The directory carries
// its own content-hash manifest so each format reconciles independently.
type Target struct {
	Format string
	OutDir string
}

// DirtySet describes what a single mutation changed, so Sync can do the minimum work. Full
// means the change could affect indexes or the inline-ref target universe (a spec/entity/
// domain/term/section-type was added, removed, or renamed) — then the only correct answer is
// a whole-graph rebuild. Otherwise the change is confined to the named specs/entities (a
// story/scenario/section/group edit, or a requirement statement edit) and only those docs
// re-render. An empty, non-Full set means nothing rendered changed (e.g. an edge add).
type DirtySet struct {
	Full      bool
	SpecIDs   []string
	EntityIDs []string
}

// empty reports whether the fast path has no docs to render.
func (d DirtySet) empty() bool { return !d.Full && len(d.SpecIDs) == 0 && len(d.EntityIDs) == 0 }

// SyncStats tallies a Sync across all targets.
type SyncStats struct {
	Full    bool     `json:"full"`
	Formats []string `json:"formats"`
	Written int      `json:"written"`
	Removed int      `json:"removed"`
}

// manifestName is the per-out-dir content-hash index that lets Sync write only files whose
// content actually changed (and, on a full rebuild, delete files whose source rows are gone).
const manifestName = ".cusp-manifest.json"

// Sync brings the configured targets up to date with the database state on x, doing the
// least work the DirtySet allows. On a Full set it rebuilds the whole graph and reconciles
// every file (including deleting orphans). On a fast set it loads only the dirty specs/
// entities and upserts just those documents. Either way it writes only files whose content
// hash changed, so an edit that touches one spec rewrites exactly one file per format.
func Sync(ctx context.Context, x store.Execer, targets []Target, dirty DirtySet) (*SyncStats, error) {
	st := &SyncStats{Full: dirty.Full}
	if len(targets) == 0 || dirty.empty() {
		return st, nil
	}

	var (
		m   *Model
		err error
	)
	if dirty.Full {
		m, err = Load(ctx, x)
	} else {
		m, err = LoadDocs(ctx, x, dirty.SpecIDs, dirty.EntityIDs)
	}
	if err != nil {
		return nil, err
	}

	for _, t := range targets {
		r, err := rendererFor(t.Format, m)
		if err != nil {
			return nil, err
		}
		files, err := r.Render(m)
		if err != nil {
			return nil, err
		}
		// On the fast path the partial model cannot produce correct index/glossary rollups,
		// so keep only the per-document outputs and never delete orphans.
		if !dirty.Full {
			files = keepDocs(files)
		}
		written, removed, err := reconcile(t.OutDir, files, dirty.Full)
		if err != nil {
			return nil, err
		}
		st.Formats = append(st.Formats, canonicalFormat(t.Format))
		st.Written += written
		st.Removed += removed
	}
	return st, nil
}

// keepDocs filters to the files the fast path can write correctly: the per-document outputs
// (spec/entity), plus static assets (the stylesheet) whose content is independent of the
// model so they reconcile to a no-op once written. Index/glossary rollups are dropped — only
// a full-graph model can render those.
func keepDocs(files []File) []File {
	out := files[:0:0]
	for _, f := range files {
		if f.Kind == "spec" || f.Kind == "entity" || f.Kind == "asset" {
			out = append(out, f)
		}
	}
	return out
}

// reconcile writes the rendered files into outDir, skipping any whose content already matches
// the manifest, and (when removeOrphans is set, i.e. a full rebuild) deletes tracked files no
// longer rendered. It then rewrites the manifest. Returns counts of files written and removed.
func reconcile(outDir string, files []File, removeOrphans bool) (written, removed int, err error) {
	old, err := loadManifest(outDir)
	if err != nil {
		return 0, 0, err
	}
	// A full rebuild starts from an empty manifest (so orphans fall out); a fast upsert keeps
	// the prior manifest and only touches the rendered docs.
	next := map[string]string{}
	if !removeOrphans {
		for k, v := range old {
			next[k] = v
		}
	}
	for _, f := range files {
		content := f.Content
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		sum := hashContent(content)
		next[f.Path] = sum
		if old[f.Path] == sum {
			continue // on disk and unchanged
		}
		if err := writeFile(outDir, f.Path, content); err != nil {
			return written, removed, err
		}
		written++
	}
	if removeOrphans {
		for p := range old {
			if _, kept := next[p]; kept {
				continue
			}
			if err := removeFile(outDir, p); err != nil {
				return written, removed, err
			}
			removed++
		}
	}
	if err := saveManifest(outDir, next); err != nil {
		return written, removed, err
	}
	return written, removed, nil
}

func hashContent(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func manifestPath(outDir string) string { return filepath.Join(outDir, manifestName) }

// loadManifest reads the content-hash index; a missing manifest is an empty index (first run).
func loadManifest(outDir string) (map[string]string, error) {
	b, err := os.ReadFile(manifestPath(outDir))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	m := map[string]string{}
	if err := json.Unmarshal(b, &m); err != nil {
		// A corrupt manifest just forces a rewrite of everything; don't fail the mutation.
		return map[string]string{}, nil
	}
	return m, nil
}

// saveManifest writes the index. encoding/json marshals string-keyed maps with sorted keys,
// so the file is deterministic and diffs cleanly.
func saveManifest(outDir string, m map[string]string) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(manifestPath(outDir), append(b, '\n'), 0o644)
}

// removeFile deletes outDir/relPath if present (orphan cleanup on a full rebuild).
func removeFile(outDir, relPath string) error {
	target := filepath.Join(outDir, filepath.FromSlash(relPath))
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
