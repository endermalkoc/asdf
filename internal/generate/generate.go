// Package generate renders the canonical database back into git-ignored, read-only build
// artifacts — the basis of Cusp's "generated, never edited" principle.
//
// Architecture: assembly and formatting are separate. Load (model.go) reads the whole
// graph once into a format-agnostic Model. A Renderer then turns that Model into a set of
// output Files; the orchestrator just writes them. Renderers fall into two families:
// document renderers (Markdown, HTML — prose pages, with inline [[TYPE:key]] refs rendered
// as links) and data serializers (JSON — structured records, refs left as canonical
// tokens). Adding a format is a new Renderer over the same Model.
package generate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/endermalkoc/cusp/internal/store"
)

// File is one rendered output document: its path (relative to the out dir, including
// extension), content, and Kind (for the Stats tally).
type File struct {
	Path    string
	Content string
	Kind    string // "spec" | "entity" | "index" | "glossary" | "planning" | "asset"
}

// Renderer turns the assembled Model into a set of output files for one format.
type Renderer interface {
	Render(m *Model) ([]File, error)
}

// rendererFor returns the renderer for a format name (md|markdown, json, html).
func rendererFor(format string, m *Model) (Renderer, error) {
	switch strings.ToLower(format) {
	case "", "md", "markdown":
		return newMarkdownRenderer(m), nil
	case "json":
		return jsonRenderer{}, nil
	case "html":
		return newHTMLRenderer(m), nil // implemented in html.go
	default:
		return nil, fmt.Errorf("unknown format %q (want: md, json, html)", format)
	}
}

// Stats tallies what was written.
type Stats struct {
	OutDir   string `json:"out_dir"`
	Format   string `json:"format"`
	Specs    int    `json:"specs"`
	Entities int    `json:"entities"`
	Indexes  int    `json:"indexes"`
	Glossary int    `json:"glossary"`
	Planning int    `json:"planning"`
}

// Total returns the file count.
func (s Stats) Total() int { return s.Specs + s.Entities + s.Indexes + s.Glossary + s.Planning }

// Generate assembles the Model from x and renders it in the given format into outDir.
// An empty format defaults to Markdown.
func Generate(ctx context.Context, x store.Execer, outDir, format string) (*Stats, error) {
	m, err := Load(ctx, x)
	if err != nil {
		return nil, err
	}
	r, err := rendererFor(format, m)
	if err != nil {
		return nil, err
	}
	files, err := r.Render(m)
	if err != nil {
		return nil, err
	}
	st := &Stats{OutDir: outDir, Format: canonicalFormat(format)}
	for _, f := range files {
		if err := writeFile(outDir, f.Path, f.Content); err != nil {
			return nil, err
		}
		switch f.Kind {
		case "spec":
			st.Specs++
		case "entity":
			st.Entities++
		case "glossary":
			st.Glossary++
		case "planning":
			st.Planning++
		default:
			st.Indexes++
		}
	}
	return st, nil
}

func canonicalFormat(format string) string {
	switch strings.ToLower(format) {
	case "", "md", "markdown":
		return "markdown"
	case "json":
		return "json"
	case "html":
		return "html"
	}
	return format
}

// writeFile writes content to outDir/relPath, creating parent directories.
func writeFile(outDir, relPath, content string) error {
	target := filepath.Join(outDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(target, []byte(content), 0o644)
}
