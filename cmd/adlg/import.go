package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/adlg/internal/app"
	"github.com/endermalkoc/adlg/internal/importer"
	"github.com/endermalkoc/adlg/internal/importer/tutor"
)

var (
	importApply   bool
	importDrizzle string
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import external corpora into ADLG",
	Long: "Import external corpora into ADLG. Today this is a read-only parse-and-report:\n" +
		"the adapter walks the source and stages an ADLG graph so it can be eyeballed\n" +
		"against the data model before any write path is wired.",
}

var importTutorCmd = &cobra.Command{
	Use:   "tutor <docs-path>",
	Short: "Parse the tutor docs corpus and report the staged graph (no writes)",
	Long: "Parse the tutor documentation corpus (the directory containing specs/ and\n" +
		"fr-registry/) into ADLG's entity shapes and print counts, a coverage histogram,\n" +
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
		printApplyStats(stats)
		return nil
	},
}

func printApplyStats(s *importer.ApplyStats) {
	var b strings.Builder
	fmt.Fprintf(&b, "imported tutor corpus\n")
	kinds := []string{"domains", "milestones", "specs", "requirement_groups", "requirements", "user_stories", "acceptance_scenarios", "entity_refs", "external_refs", "entities", "entity_relationships", "sections"}
	fmt.Fprintf(&b, "  %-22s %8s %8s %8s\n", "", "inserted", "updated", "skipped")
	for _, k := range kinds {
		if s.Inserted[k] == 0 && s.Updated[k] == 0 && s.Skipped[k] == 0 {
			continue
		}
		fmt.Fprintf(&b, "  %-22s %8d %8d %8d\n", k, s.Inserted[k], s.Updated[k], s.Skipped[k])
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

func init() {
	importTutorCmd.Flags().StringVar(&importDrizzle, "drizzle", "",
		"path to the tutor Drizzle schema dir for entity relationships (default: auto-detect at <docs>/../src/packages/database/src/schema)")
	importTutorCmd.Flags().BoolVar(&importApply, "apply", false,
		"write the parsed graph into the database via the command contract (default: read-only report)")
	importCmd.AddCommand(importTutorCmd)
	rootCmd.AddCommand(importCmd)
}
