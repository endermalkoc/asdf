package tutor

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/endermalkoc/cusp/internal/importer"
)

// drizzle.go extracts entity relationships from the tutor app's Drizzle schema
// (TypeScript, one pgTable per export). The ground truth is the foreign key —
// `.references(() => Target.id)` — not the duplicative `relations()` DSL:
//   - FK on the `id` column          → one-to-one  (shared PK; e.g. Student ↔ Attachable)
//   - FK on any other column         → one-to-many (referenced = the "one", holder = "many")
//   - a non-entity table whose composite PK is exactly two FK columns → many-to-many
//     between the two referenced entities, with junction_table set.
// Only relationships where BOTH endpoints map to one of our entities are emitted;
// N-ary (3+ FK) junctions can't be expressed by the binary table and are reported.

var (
	reTableStart  = regexp.MustCompile(`export const (\w+) = pgTable\(`)
	reInlineSnake = regexp.MustCompile(`pgTable\(\s*"([a-z0-9_]+)"`)
	reSnakeLine   = regexp.MustCompile(`^\s*"([a-z0-9_]+)",`)
	reColLine     = regexp.MustCompile(`^\s+(\w+):`)
	reFKRef       = regexp.MustCompile(`\.references\(\(\) => (\w+)\.id`)
	rePK          = regexp.MustCompile(`primaryKey\(\{\s*columns:\s*\[([^\]]+)\]`)
	rePKCol       = regexp.MustCompile(`table\.(\w+)`)
	reCamelBound  = regexp.MustCompile(`([a-z0-9])([A-Z])`)
)

type drizzleFK struct{ col, target string }

type drizzleTable struct {
	camel  string
	snake  string
	fks    []drizzleFK
	pkCols []string
}

// kebab lower-cases a camelCase identifier with dashes at the boundaries
// ("studioOptions" → "studio-options").
func kebab(s string) string {
	return strings.ToLower(reCamelBound.ReplaceAllString(s, "$1-$2"))
}

// singular naively singularizes a kebab table name ("studio-options" →
// "studio-option", "families" → "family").
func singular(s string) string {
	switch {
	case strings.HasSuffix(s, "ies"):
		return s[:len(s)-3] + "y"
	case strings.HasSuffix(s, "ss"):
		return s
	case strings.HasSuffix(s, "s"):
		return s[:len(s)-1]
	default:
		return s
	}
}

// parseDrizzle reads schemaDir/*.ts and returns the entity relationships it can map
// onto entityNames (the canonical entity names from the docs corpus).
func parseDrizzle(schemaDir string, entityNames []string, rep *importer.Report) []importer.EntityRelationship {
	byKey := make(map[string]string, len(entityNames)) // kebab(name) → canonical name
	for _, n := range entityNames {
		byKey[kebab(n)] = n
	}
	resolve := func(camel string) string { return byKey[singular(kebab(camel))] }

	files, _ := filepath.Glob(filepath.Join(schemaDir, "*.ts"))

	var rels []importer.EntityRelationship
	seen := map[string]bool{}
	add := func(from, to, card, junction string) {
		if from == "" || to == "" || from == to {
			return
		}
		k := from + "\x1f" + to + "\x1f" + card + "\x1f" + junction
		if seen[k] {
			return
		}
		seen[k] = true
		rels = append(rels, importer.EntityRelationship{FromName: from, ToName: to, Cardinality: card, JunctionTable: junction})
	}

	var ternary []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, t := range parseDrizzleTables(string(b)) {
			if ent := resolve(t.camel); ent != "" {
				// Entity table → its FKs are 1:1 (on id) or 1:N relationships.
				for _, fk := range t.fks {
					target := resolve(fk.target)
					if target == "" {
						continue
					}
					if fk.col == "id" {
						add(ent, target, "one_to_one", "")
					} else {
						add(target, ent, "one_to_many", "") // target = the "one" side
					}
				}
				continue
			}
			// Non-entity table with a composite PK of FK columns → junction (m2m).
			if len(t.pkCols) < 2 {
				continue
			}
			pk := map[string]bool{}
			for _, c := range t.pkCols {
				pk[c] = true
			}
			var ents []string
			for _, fk := range t.fks {
				if pk[fk.col] {
					if e := resolve(fk.target); e != "" && !contains(ents, e) {
						ents = append(ents, e)
					}
				}
			}
			switch {
			case len(ents) == 2:
				add(ents[0], ents[1], "many_to_many", t.snake)
			case len(ents) > 2:
				ternary = append(ternary, t.snake)
			}
		}
	}
	sort.Slice(rels, func(i, j int) bool {
		if rels[i].FromName != rels[j].FromName {
			return rels[i].FromName < rels[j].FromName
		}
		return rels[i].ToName < rels[j].ToName
	})
	if len(rels) > 0 {
		rep.Add(importer.SevInfo, "drizzle-relationships",
			itoa(len(rels))+" entity relationships extracted from the Drizzle schema (FK + junction → ent_relationship)", "")
	}
	if len(ternary) > 0 {
		sort.Strings(ternary)
		rep.Add(importer.SevInfo, "drizzle-ternary-junction",
			itoa(len(ternary))+" N-ary junction(s) skipped (binary ent_relationship can't represent): "+strings.Join(ternary, ", "), "")
	}
	return rels
}

// parseDrizzleTables splits a schema file's pgTable definitions (a file may hold
// more than one) into their camel/snake names, FK columns, and composite-PK columns.
func parseDrizzleTables(content string) []drizzleTable {
	var out []drizzleTable
	var cur *drizzleTable
	var curCol string
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(content, "\n") {
		if m := reTableStart.FindStringSubmatch(line); m != nil {
			flush()
			cur = &drizzleTable{camel: m[1]}
			curCol = ""
			if sm := reInlineSnake.FindStringSubmatch(line); sm != nil {
				cur.snake = sm[1]
			}
			continue
		}
		if cur == nil {
			continue
		}
		// Any later top-level `export const` (e.g. the relations() block) ends the table.
		if strings.HasPrefix(line, "export const ") {
			flush()
			continue
		}
		if cur.snake == "" {
			if m := reSnakeLine.FindStringSubmatch(line); m != nil {
				cur.snake = m[1]
				continue
			}
		}
		if m := reColLine.FindStringSubmatch(line); m != nil {
			curCol = m[1]
		}
		if m := reFKRef.FindStringSubmatch(line); m != nil {
			cur.fks = append(cur.fks, drizzleFK{col: curCol, target: m[1]})
		}
		if m := rePK.FindStringSubmatch(line); m != nil {
			for _, c := range rePKCol.FindAllStringSubmatch(m[1], -1) {
				cur.pkCols = append(cur.pkCols, c[1])
			}
		}
	}
	flush()
	return out
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
