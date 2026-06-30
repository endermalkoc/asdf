package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/importer"
	"github.com/endermalkoc/cusp/internal/importer/notion"
	"github.com/endermalkoc/cusp/internal/importer/tutor"
)

var (
	importApply   bool
	importDrizzle string

	notionToken   string
	notionFrom    string
	notionCapDB   string
	notionDelivDB string
	notionViewsDB string
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import external corpora into Cusp",
	Long: "Import external corpora into Cusp. Today this is a read-only parse-and-report:\n" +
		"the adapter walks the source and stages a Cusp graph so it can be eyeballed\n" +
		"against the data model before any write path is wired.",
}

var importTutorCmd = &cobra.Command{
	Use:   "tutor <docs-path>",
	Short: "Parse the tutor docs corpus and report the staged graph (no writes)",
	Long: "Parse the tutor documentation corpus (the directory containing specs/ and\n" +
		"fr-registry/) into Cusp's entity shapes and print counts, a coverage histogram,\n" +
		"and drift / ER-gap findings. This is deterministic and read-only — it never\n" +
		"connects to the database. Use --json to emit the full staged graph + report.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		g, rep, err := tutor.Parse(args[0], importDrizzle)
		if err != nil {
			return err
		}

		if !importApply {
			// Read-only: report only.
			if flagJSON {
				out := struct {
					Graph  *importer.Graph  `json:"graph"`
					Report *importer.Report `json:"report"`
				}{g, rep}
				b, _ := json.MarshalIndent(out, "", "  ")
				fmt.Println(string(b))
				return nil
			}
			printTutorReport(args[0], g, rep)
			return nil
		}

		// Write path: load the staged graph through the command contract — one
		// transaction, one Dolt commit, on the changeset branch or main.
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		var stats *importer.ApplyStats
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: fmt.Sprintf("import tutor corpus (%d specs, %d requirements)", rep.Counts["specs"], rep.Counts["requirements"]),
		}, func(ctx context.Context, w *app.Write) error {
			s, e := importer.Apply(ctx, w.Tx, g)
			if e != nil {
				return e
			}
			stats = s
			for _, t := range importer.TouchedTables {
				w.MarkDirty(t)
			}
			return nil
		})
		if err != nil {
			return err
		}
		if flagJSON {
			emit(stats, "")
			return nil
		}
		printApplyStats("imported tutor corpus", tutorKinds, stats)
		return nil
	},
}

var tutorKinds = []string{"domains", "milestones", "specs", "requirement_groups", "requirements", "user_stories", "acceptance_scenarios", "entity_refs", "external_refs", "entities", "entity_relationships", "sections"}

var planningKinds = []string{"domains", "milestones", "capabilities", "deliverables", "views", "capability_milestone", "capability_deliverable", "deliverable_view", "deliverable_dependency", "external_refs"}

func printApplyStats(title string, kinds []string, s *importer.ApplyStats) {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", title)
	fmt.Fprintf(&b, "  %-24s %8s %8s %8s\n", "", "inserted", "updated", "skipped")
	for _, k := range kinds {
		if s.Inserted[k] == 0 && s.Updated[k] == 0 && s.Skipped[k] == 0 {
			continue
		}
		fmt.Fprintf(&b, "  %-24s %8d %8d %8d\n", k, s.Inserted[k], s.Updated[k], s.Skipped[k])
	}
	fmt.Print(b.String())
}

var importNotionCmd = &cobra.Command{
	Use:   "notion",
	Short: "Import a Notion planning workspace (capabilities, deliverables, views)",
	Long: "Import the Notion planning databases (Capabilities / Deliverables / Views) into\n" +
		"Cusp's planning layer. By default this is a read-only parse-and-report; --apply\n" +
		"writes the staged graph through the command contract (one transaction, one Dolt\n" +
		"commit), idempotent on re-run.\n\n" +
		"Source: the Notion API (--token, or $NOTION_API_KEY / $CUSP_NOTION_TOKEN), or saved\n" +
		"query responses with --from <dir> (capabilities.json / deliverables.json / views.json).",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()

		var (
			g   *importer.Graph
			rep *importer.Report
			err error
		)
		if notionFrom != "" {
			g, rep, err = notion.ParseDir(notionFrom)
		} else {
			token := notionToken
			if token == "" {
				token = envFirst("CUSP_NOTION_TOKEN", "NOTION_API_KEY")
			}
			g, rep, err = notion.Parse(ctx, notion.Config{
				Token:          token,
				CapabilitiesDB: notionCapDB,
				DeliverablesDB: notionDelivDB,
				ViewsDB:        notionViewsDB,
			})
		}
		if err != nil {
			return err
		}

		if !importApply {
			if flagJSON {
				out := struct {
					Graph  *importer.Graph  `json:"graph"`
					Report *importer.Report `json:"report"`
				}{g, rep}
				b, _ := json.MarshalIndent(out, "", "  ")
				fmt.Println(string(b))
				return nil
			}
			printNotionReport(g, rep)
			return nil
		}

		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		var stats *importer.ApplyStats
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: fmt.Sprintf("import Notion planning (%d capabilities, %d deliverables, %d views)",
				rep.Counts["capabilities"], rep.Counts["deliverables"], rep.Counts["views"]),
		}, func(ctx context.Context, w *app.Write) error {
			s, e := importer.Apply(ctx, w.Tx, g)
			if e != nil {
				return e
			}
			stats = s
			for _, t := range importer.TouchedTables {
				w.MarkDirty(t)
			}
			return nil
		})
		if err != nil {
			return err
		}
		if flagJSON {
			emit(stats, "")
			return nil
		}
		printApplyStats("imported Notion planning workspace", planningKinds, stats)
		return nil
	},
}

func printNotionReport(g *importer.Graph, rep *importer.Report) {
	var b strings.Builder
	fmt.Fprintf(&b, "Notion planning workspace\n\nparsed entities\n")
	for _, k := range []string{"domains", "milestones", "capabilities", "deliverables", "views"} {
		fmt.Fprintf(&b, "  %-14s %d\n", k, rep.Counts[k])
	}
	order := map[string]int{importer.SevGap: 0, importer.SevWarn: 1, importer.SevInfo: 2}
	label := map[string]string{importer.SevGap: "ER GAPS", importer.SevWarn: "WARNINGS", importer.SevInfo: "notes"}
	findings := append([]importer.Finding(nil), rep.Findings...)
	sort.SliceStable(findings, func(i, j int) bool {
		if order[findings[i].Severity] != order[findings[j].Severity] {
			return order[findings[i].Severity] < order[findings[j].Severity]
		}
		return findings[i].Category < findings[j].Category
	})
	lastSev := ""
	for _, f := range findings {
		if f.Severity != lastSev {
			fmt.Fprintf(&b, "\n%s\n", label[f.Severity])
			lastSev = f.Severity
		}
		ref := ""
		if f.Ref != "" {
			ref = "  (" + f.Ref + ")"
		}
		fmt.Fprintf(&b, "  [%s] %s%s\n", f.Category, f.Message, ref)
	}
	if len(findings) == 0 {
		fmt.Fprintf(&b, "\nno findings\n")
	}
	fmt.Print(b.String())
}

func printTutorReport(path string, g *importer.Graph, rep *importer.Report) {
	var b strings.Builder
	fmt.Fprintf(&b, "tutor corpus: %s\n", path)

	fmt.Fprintf(&b, "\nparsed entities\n")
	for _, k := range []string{"domains", "specs", "requirements", "user_stories", "acceptance_scenarios", "entity_refs", "milestones", "entities"} {
		fmt.Fprintf(&b, "  %-22s %d\n", k, rep.Counts[k])
	}

	fmt.Fprintf(&b, "\ndelivery coverage\n")
	type kv struct {
		k string
		v int
	}
	var cov []kv
	for k, v := range rep.Coverage {
		cov = append(cov, kv{k, v})
	}
	sort.Slice(cov, func(i, j int) bool { return cov[i].v > cov[j].v })
	for _, c := range cov {
		fmt.Fprintf(&b, "  %-18s %d\n", c.k, c.v)
	}

	// Findings grouped by severity.
	order := map[string]int{importer.SevGap: 0, importer.SevWarn: 1, importer.SevInfo: 2}
	label := map[string]string{importer.SevGap: "ER GAPS", importer.SevWarn: "DRIFT (warnings)", importer.SevInfo: "notes"}
	findings := append([]importer.Finding(nil), rep.Findings...)
	sort.SliceStable(findings, func(i, j int) bool {
		if order[findings[i].Severity] != order[findings[j].Severity] {
			return order[findings[i].Severity] < order[findings[j].Severity]
		}
		return findings[i].Category < findings[j].Category
	})
	lastSev := ""
	for _, f := range findings {
		if f.Severity != lastSev {
			fmt.Fprintf(&b, "\n%s\n", label[f.Severity])
			lastSev = f.Severity
		}
		ref := ""
		if f.Ref != "" {
			ref = "  (" + f.Ref + ")"
		}
		fmt.Fprintf(&b, "  [%s] %s%s\n", f.Category, f.Message, ref)
	}
	if len(findings) == 0 {
		fmt.Fprintf(&b, "\nno findings\n")
	}
	fmt.Print(b.String())
}

// envFirst returns the value of the first set (non-empty) environment variable.
func envFirst(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func init() {
	importTutorCmd.Flags().StringVar(&importDrizzle, "drizzle", "",
		"path to the tutor Drizzle schema dir for entity relationships (default: auto-detect at <docs>/../src/packages/database/src/schema)")
	importTutorCmd.Flags().BoolVar(&importApply, "apply", false,
		"write the parsed graph into the database via the command contract (default: read-only report)")
	importCmd.AddCommand(importTutorCmd)

	importNotionCmd.Flags().StringVar(&notionToken, "token", "", "Notion integration token (default: $CUSP_NOTION_TOKEN, then $NOTION_API_KEY)")
	importNotionCmd.Flags().StringVar(&notionFrom, "from", "", "import from saved query responses in this dir (capabilities.json/deliverables.json/views.json) instead of the API")
	importNotionCmd.Flags().StringVar(&notionCapDB, "capabilities-db", "", "override the Capabilities database id")
	importNotionCmd.Flags().StringVar(&notionDelivDB, "deliverables-db", "", "override the Deliverables database id")
	importNotionCmd.Flags().StringVar(&notionViewsDB, "views-db", "", "override the Views database id")
	importNotionCmd.Flags().BoolVar(&importApply, "apply", false,
		"write the parsed graph into the database via the command contract (default: read-only report)")
	importCmd.AddCommand(importNotionCmd)

	rootCmd.AddCommand(importCmd)
}
