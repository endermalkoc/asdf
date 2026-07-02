package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Workspace health in one report: integrity, coverage, and hygiene",
	Long: "Roll up the workspace's health checks: integrity (unresolved [[TYPE:key]] references and\n" +
		"edge cycles — the same as `cusp check`), requirement→test-case coverage (orphan FRs and\n" +
		"delivery-status drift — summarized from `cusp coverage`), and structural hygiene (empty\n" +
		"domains, FR-bearing specs with no requirements). Read-only; honors the active changeset.\n" +
		"Exits nonzero when there are integrity problems (so it can gate CI or an agent step);\n" +
		"coverage gaps and hygiene warnings are informational.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		r, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		rep, err := app.Doctor(ctx, r)
		if err != nil {
			return err
		}
		if flagJSON {
			emit(rep, "")
		} else {
			fmt.Print(renderDoctor(rep))
		}
		// The report (its "✗" verdict / the integrity array) already conveys the problems; gate via
		// the exit code without a second error line or JSON envelope.
		if !rep.Healthy() {
			return app.ExitWith(app.ExitGeneric)
		}
		return nil
	},
}

func renderDoctor(rep app.DoctorReport) string {
	var b strings.Builder
	b.WriteString("cusp doctor — workspace health\n\n")

	if len(rep.Integrity) == 0 {
		b.WriteString("integrity: clean\n")
	} else {
		fmt.Fprintf(&b, "integrity: %d problem(s)\n", len(rep.Integrity))
		for _, f := range rep.Integrity {
			fmt.Fprintf(&b, "  [%s] %s: %s\n", f.Kind, f.Location, f.Detail)
		}
	}

	c := rep.Coverage
	fmt.Fprintf(&b, "coverage:  %d/%d requirements have a test case (%d%%)\n", c.Covered, c.Total, pct(c.Covered, c.Total))
	if c.Orphans > 0 || c.Drift > 0 {
		fmt.Fprintf(&b, "  orphan FRs: %d · delivery-status drift: %d  (see `cusp coverage`)\n", c.Orphans, c.Drift)
	}

	if len(rep.Hygiene) == 0 {
		b.WriteString("hygiene:   clean\n")
	} else {
		fmt.Fprintf(&b, "hygiene:   %d warning(s)\n", len(rep.Hygiene))
		for _, h := range rep.Hygiene {
			fmt.Fprintf(&b, "  [%s] %s\n", h.Kind, h.Location)
		}
	}

	b.WriteString("\n")
	if rep.Healthy() {
		b.WriteString("✓ healthy\n")
	} else {
		fmt.Fprintf(&b, "✗ %d integrity problem(s)\n", len(rep.Integrity))
	}
	return b.String()
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
