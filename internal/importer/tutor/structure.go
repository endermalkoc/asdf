package tutor

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/endermalkoc/cusp/internal/importer"
)

// ---- frontmatter -----------------------------------------------------------

type frontmatter struct {
	ID     string `yaml:"id"`
	Title  string `yaml:"title"`
	Domain string `yaml:"domain"`
	Status string `yaml:"status"`
}

// readFrontmatter extracts and parses a leading `---`-delimited YAML block.
func readFrontmatter(absPath string) (frontmatter, bool) {
	b, err := os.ReadFile(absPath)
	if err != nil {
		return frontmatter{}, false
	}
	block, ok := frontmatterBlock(string(b))
	if !ok {
		return frontmatter{}, false
	}
	var fm frontmatter
	if err := yaml.Unmarshal([]byte(block), &fm); err != nil {
		return frontmatter{}, false
	}
	return fm, fm.ID != "" || fm.Title != "" || fm.Domain != "" || fm.Status != ""
}

// frontmatterBlock returns the YAML between the opening and closing `---` lines.
func frontmatterBlock(s string) (string, bool) {
	lines := strings.Split(s, "\n")
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) || strings.TrimSpace(lines[i]) != "---" {
		return "", false
	}
	var body []string
	for j := i + 1; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == "---" {
			return strings.Join(body, "\n"), true
		}
		body = append(body, lines[j])
	}
	return "", false
}

// ---- markdown tables -------------------------------------------------------

// tableCells splits a `| a | b |` row into trimmed cells. ok=false for a
// non-table line or a `---|---` separator row.
func tableCells(line string) ([]string, bool) {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "|") {
		return nil, false
	}
	t = strings.Trim(t, "|")
	parts := strings.Split(t, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	sep := true
	for _, c := range parts {
		if strings.Trim(c, "-: ") != "" {
			sep = false
			break
		}
	}
	if sep {
		return nil, false
	}
	return parts, true
}

var linkTextRe = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)

// firstLinkText returns the text of the first markdown link in s, else s itself.
func firstLinkText(s string) string {
	if m := linkTextRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return s
}

// ---- domains (specs/index.md) ----------------------------------------------

// parseDomains reads the two domain tables in specs/index.md. The directory
// name is the slug; kind is inferred from well-known reference dirs.
func parseDomains(indexPath string) ([]importer.Domain, error) {
	b, err := os.ReadFile(indexPath)
	if err != nil {
		// index.md is optional; fall back to whatever the spec table reveals.
		return nil, nil
	}
	var out []importer.Domain
	seen := map[string]bool{}
	for _, line := range strings.Split(string(b), "\n") {
		cells, ok := tableCells(line)
		if !ok || len(cells) < 2 {
			continue
		}
		head := strings.ToLower(firstLinkText(cells[0]))
		if head == "domain" || head == "directory" {
			continue // header row
		}
		slug := strings.TrimSuffix(strings.TrimSpace(firstLinkText(cells[0])), "/")
		if slug == "" || strings.Contains(slug, " ") || seen[slug] {
			continue
		}
		// "entities" is not a real domain — entity docs are domain-less first-class
		// documents (their folder is doc organization, not a domain).
		if slug == "entities" {
			continue
		}
		seen[slug] = true
		out = append(out, importer.Domain{
			Slug:        slug,
			Name:        titleCase(slug),
			Description: cells[1],
		})
	}
	return out, nil
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ---- specs (specs/prefix-registry.md) --------------------------------------

// parseSpecs reads the prefix registry table (the authoritative spec list) and
// enriches each row from the spec file's frontmatter. It records drift findings
// (missing files, domain mismatches, unregistered specs) into rep.
func parseSpecs(specsDir string, rep *importer.Report, domainSet map[string]bool) ([]importer.Spec, map[string]string, error) {
	regPath := filepath.Join(specsDir, "prefix-registry.md")
	b, err := os.ReadFile(regPath)
	if err != nil {
		return nil, nil, err
	}
	var specs []importer.Spec
	fmStatus := map[string]string{} // prefix → mapped status
	registered := map[string]bool{} // path → registered

	for _, line := range strings.Split(string(b), "\n") {
		cells, ok := tableCells(line)
		if !ok || len(cells) < 3 {
			continue
		}
		if strings.EqualFold(cells[0], "prefix") {
			continue // header
		}
		prefix := strings.TrimSpace(cells[0])
		path := strings.TrimSpace(firstLinkText(cells[1]))
		domain := strings.TrimSpace(cells[2])
		regStatus := ""
		if len(cells) >= 4 {
			regStatus = strings.TrimSpace(cells[3])
		}
		if prefix == "" || path == "" {
			continue
		}
		// Docs under entities/ are entities (ent_entity), not specs — even if the
		// registry lists a prefix for them (e.g. ATCH/attachable, which has no FRs).
		if strings.HasPrefix(path, "entities/") {
			continue
		}
		registered[path] = true

		sp := importer.Spec{
			Prefix: prefix,
			Path:   path, // full source path (used for disk reads + dedup); apply stores it domain-relative
			Domain: domain,
		}

		// Enrich from frontmatter.
		fm, hasFM := readFrontmatter(filepath.Join(specsDir, path))
		if _, statErr := os.Stat(filepath.Join(specsDir, path)); statErr != nil {
			rep.Add(importer.SevWarn, "registry-missing-file",
				"prefix-registry lists a spec file that does not exist on disk", path)
		}
		if hasFM {
			sp.Title = fm.Title
			if fm.ID != "" && fm.ID != prefix {
				rep.Add(importer.SevWarn, "prefix-mismatch",
					"frontmatter id "+fm.ID+" differs from registry prefix "+prefix, path)
			}
			sp.RawStatus = fm.Status
		}
		if sp.RawStatus == "" {
			sp.RawStatus = regStatus
		}
		sp.Status = mapSpecStatus(sp.RawStatus, rep, path)
		fmStatus[prefix] = sp.Status

		// Domain checks.
		if domain != "" && !domainSet[domain] {
			rep.Add(importer.SevWarn, "unknown-domain",
				"spec domain "+domain+" is not a known domain directory", path)
		}
		if topDir := topDir(path); domain != "" && topDir != "" && topDir != domain {
			rep.Add(importer.SevInfo, "cross-domain-spec",
				"spec file lives under "+topDir+"/ but is tagged domain "+domain, path)
		}

		specs = append(specs, sp)
	}

	// Specs with frontmatter id that are NOT in the registry → drift.
	_ = filepath.WalkDir(specsDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".md" {
			return nil
		}
		rel, _ := filepath.Rel(specsDir, p)
		if registered[rel] {
			return nil
		}
		if fm, ok := readFrontmatter(p); ok && fm.ID != "" {
			rep.Add(importer.SevWarn, "unregistered-spec",
				"spec has frontmatter id "+fm.ID+" but is absent from prefix-registry", rel)
		}
		return nil
	})

	return specs, fmStatus, nil
}

func topDir(relPath string) string {
	if i := strings.IndexByte(relPath, '/'); i >= 0 {
		return relPath[:i]
	}
	return ""
}

// mapSpecStatus maps a tutor spec status (Draft|Reviewed|Active|Retired…) to
// Cusp's spec.status enum (draft|reviewed|active|obsolete). `reviewed` is a
// first-class value (added in migration 0001's enum set per decisions.md), so it
// is preserved, not downgraded.
func mapSpecStatus(raw string, rep *importer.Report, ref string) string {
	r := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case r == "":
		return "draft"
	case strings.HasPrefix(r, "retired"), strings.HasPrefix(r, "obsolete"), strings.HasPrefix(r, "superseded"):
		return "obsolete"
	case r == "active":
		return "active"
	case r == "reviewed":
		return "reviewed"
	case r == "draft":
		return "draft"
	default:
		return "draft"
	}
}
