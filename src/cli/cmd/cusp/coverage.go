package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
)

var coverageOrphansOnly bool

var coverageCmd = &cobra.Command{
	Use:   "coverage",
	Short: "Requirement→test-case coverage: rollups, orphan FRs, and delivery-status drift",
	Long: "Report how well requirements are backed by test cases (via the req_requirement_test_case\n" +
		"links, e.g. from `cusp import qase`): the overall and per-spec covered ratio, the orphan\n" +
		"FRs with no test case, and delivery-status drift — requirements whose delivery status\n" +
		"counts as covered yet have no test case behind it. Read-only; honors the active changeset.\n\n" +
		"  cusp coverage             # rollups + gaps\n" +
		"  cusp coverage --orphans   # just the untested FRs\n" +
		"  cusp coverage --json",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		r, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		rep, err := app.Coverage(ctx, r)
		if err != nil {
			return err
		}
		if flagJSON {
			emit(rep, "")
			return nil
		}
		fmt.Print(renderCoverage(rep, coverageOrphansOnly))
		return nil
	},
}

func renderCoverage(rep app.CoverageReport, orphansOnly bool) string {
	var b strings.Builder
	if orphansOnly {
		writeOrphanList(&b, rep.Orphans)
		return b.String()
	}
	fmt.Fprintf(&b, "coverage: %d/%d requirements have a test case (%d%%)\n",
		rep.Covered, rep.Total, pct(rep.Covered, rep.Total))
	if len(rep.BySpec) > 0 {
		b.WriteString("\nby spec:\n")
		for _, s := range rep.BySpec {
			fmt.Fprintf(&b, "  %-12s %d/%-4d (%d%%)\n", s.Prefix, s.Covered, s.Total, pct(s.Covered, s.Total))
		}
	}
	b.WriteString("\n")
	writeOrphanList(&b, rep.Orphans)
	if len(rep.Drift) > 0 {
		fmt.Fprintf(&b, "\ndelivery-status drift (counts as covered, but no test case): %d\n", len(rep.Drift))
		for _, c := range rep.Drift {
			fmt.Fprintf(&b, "  %-16s %s\n", c.FRKey, c.DeliveryStatus)
		}
	}
	return b.String()
}

func writeOrphanList(b *strings.Builder, orphans []app.CoverageReq) {
	if len(orphans) == 0 {
		b.WriteString("no orphan FRs — every requirement has a test case\n")
		return
	}
	fmt.Fprintf(b, "orphan FRs (no test case): %d\n", len(orphans))
	for _, c := range orphans {
		status := c.DeliveryStatus
		if status == "" {
			status = "-"
		}
		fmt.Fprintf(b, "  %-16s %s\n", c.FRKey, status)
	}
}

func pct(n, total int) int {
	if total == 0 {
		return 0
	}
	return 100 * n / total
}

func init() {
	coverageCmd.Flags().BoolVar(&coverageOrphansOnly, "orphans", false, "list only the requirements with no test case")
	rootCmd.AddCommand(coverageCmd)
}
