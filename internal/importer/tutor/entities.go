package tutor

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/endermalkoc/cusp/internal/importer"
)

// parseEntities reads specs/entities/index.md (the curated entity glossary table:
// `Entity | Description | Status`) into Entity rows. Entities are first-class
// documents (ent_entity), not specs — each carries its own doc path.
// Entity .md files present on disk but absent from the index are flagged as drift.
func parseEntities(specsDir string, rep *importer.Report) []importer.Entity {
	indexPath := filepath.Join(specsDir, "entities", "index.md")
	b, err := os.ReadFile(indexPath)
	if err != nil {
		return nil
	}
	var entities []importer.Entity
	listed := map[string]bool{} // entities/<file>.md → in index

	for _, line := range strings.Split(string(b), "\n") {
		cells, ok := tableCells(line)
		if !ok || len(cells) < 2 {
			continue
		}
		if strings.EqualFold(firstLinkText(cells[0]), "entity") {
			continue // header
		}
		name := strings.TrimSpace(firstLinkText(cells[0]))
		href := linkHref(cells[0])
		if name == "" || href == "" {
			continue
		}
		rel := filepath.ToSlash(filepath.Join("entities", strings.TrimPrefix(href, "./")))
		listed[rel] = true
		status := ""
		if len(cells) >= 3 {
			status = strings.TrimSpace(cells[2])
		}
		mapped := mapSpecStatus(status, rep, rel)
		ent := importer.Entity{Name: name, Description: strings.TrimSpace(cells[1]), Status: mapped, DocPath: rel}
		if body, ok := readSpecBody(specsDir, rel); ok {
			_, preamble, secs := splitDoc(body)
			routeEntitySections(&ent, preamble, secs)
		}
		entities = append(entities, ent)
	}

	// Entity docs on disk but not in the index are imported too — the index is a
	// curation hint (name + description + status), not the gate on existence. An
	// unlisted doc derives its name from its H1 and carries no index description.
	unlisted := 0
	entriesDir := filepath.Join(specsDir, "entities")
	ents, _ := os.ReadDir(entriesDir)
	for _, e := range ents {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" || e.Name() == "index.md" {
			continue
		}
		rel := "entities/" + e.Name()
		if listed[rel] {
			continue
		}
		unlisted++
		body, ok := readSpecBody(specsDir, rel)
		if !ok {
			continue
		}
		h1, preamble, secs := splitDoc(body)
		name := entityNameFromHeading(h1, e.Name())
		mapped := mapSpecStatus("", rep, rel)
		ent := importer.Entity{Name: name, Status: mapped, DocPath: rel}
		routeEntitySections(&ent, preamble, secs)
		entities = append(entities, ent)
	}
	if unlisted > 0 {
		rep.Add(importer.SevInfo, "entity-doc-unlisted",
			itoa(unlisted)+" entity docs not in entities/index.md were imported from disk (index is a curation hint)", "")
	}
	return entities
}

// entityNameFromHeading derives an entity's display name from its doc H1
// ("# Invoice Entity" → "Invoice"), falling back to a title-cased filename
// ("transaction-recurring-schedule.md" → "Transaction Recurring Schedule").
func entityNameFromHeading(h1, filename string) string {
	name := strings.TrimSpace(strings.TrimLeft(h1, "# "))
	name = strings.TrimSpace(strings.TrimSuffix(name, "Entity"))
	if name != "" {
		return name
	}
	base := strings.TrimSuffix(filename, ".md")
	parts := strings.FieldsFunc(base, func(r rune) bool { return r == '-' || r == '_' })
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

var linkHrefRe = regexp.MustCompile(`\[[^\]]+\]\(([^)]*)\)`)

// linkHref returns the URL of the first markdown link in s (empty if none).
func linkHref(s string) string {
	if m := linkHrefRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}
