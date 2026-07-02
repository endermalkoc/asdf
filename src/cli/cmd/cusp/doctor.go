package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/workspace"
)

var doctorFix bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Workspace health in one report: integrity, schema/data drift, coverage, hygiene",
	Long: "Roll up the workspace's health checks: integrity (unresolved [[TYPE:key]] references and\n" +
		"edge cycles), schema drift (the database's applied migration version vs this cusp's),\n" +
		"requirement→test-case coverage (orphan FRs + delivery-status drift), fr_key data drift, and\n" +
		"structural hygiene (empty domains, FR-bearing specs with no requirements). Read-only by\n" +
		"default; honors the active changeset. Exits nonzero on integrity problems or schema drift\n" +
		"(so it can gate CI/an agent); coverage, hygiene, and fr_key drift are informational.\n\n" +
		"  cusp doctor          # report\n" +
		"  cusp doctor --fix    # apply the auto-fixable drift (migrate a behind DB; recompute fr_keys)",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if doctorFix {
			return runDoctorFix(ctx)
		}
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
		if !rep.Healthy() {
			return app.ExitWith(app.ExitGeneric)
		}
		return nil
	},
}

// runDoctorFix applies the auto-fixable drift (schema migration if behind, fr_key recompute), then
// re-reports. Forward schema drift (the DB is ahead of this cusp) cannot be fixed here.
func runDoctorFix(ctx context.Context) error {
	ws, err := connect(ctx)
	if err != nil {
		return err
	}
	defer ws.Close()

	migrated, err := app.MigrateWorkspace(ctx, ws, workspace.ResolveActor(flagActor))
	if err != nil {
		return err
	}
	fixedKeys, err := app.FixFRKeyDrift(ctx, ws, app.MutateOpts{Changeset: "main", Actor: flagActor})
	if err != nil {
		return err
	}

	r, release, err := app.Reader(ctx, ws, "main")
	if err != nil {
		return err
	}
	rep, err := app.Doctor(ctx, r)
	_ = release()
	if err != nil {
		return err
	}

	if flagJSON {
		emit(map[string]any{"migrated": migrated, "fixedFRKeys": fixedKeys, "report": rep}, "")
	} else {
		var b strings.Builder
		b.WriteString("cusp doctor --fix\n\n")
		if len(migrated) == 0 && fixedKeys == 0 {
			b.WriteString("nothing to fix\n\n")
		} else {
			if len(migrated) > 0 {
				fmt.Fprintf(&b, "✓ migrated schema → v%d (%d migration(s) applied)\n", rep.Schema.Current, len(migrated))
			}
			if fixedKeys > 0 {
				fmt.Fprintf(&b, "✓ recomputed %d drifted fr_key(s)\n", fixedKeys)
			}
			b.WriteString("\n")
		}
		b.WriteString(renderDoctor(rep))
		fmt.Print(b.String())
	}
	if !rep.Healthy() {
		return app.ExitWith(app.ExitGeneric)
	}
	return nil
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

	s := rep.Schema
	switch s.Status {
	case "behind":
		fmt.Fprintf(&b, "schema:    BEHIND — database v%d < cusp v%d (%d pending) — run `cusp doctor --fix`\n", s.Current, s.Latest, s.Pending)
	case "ahead":
		fmt.Fprintf(&b, "schema:    AHEAD — database v%d > cusp v%d — upgrade cusp before writing\n", s.Current, s.Latest)
	default:
		fmt.Fprintf(&b, "schema:    up to date (v%d)\n", s.Current)
	}

	c := rep.Coverage
	fmt.Fprintf(&b, "coverage:  %d/%d requirements have a test case (%d%%)\n", c.Covered, c.Total, pct(c.Covered, c.Total))
	if c.Orphans > 0 || c.Drift > 0 {
		fmt.Fprintf(&b, "  orphan FRs: %d · delivery-status drift: %d  (see `cusp coverage`)\n", c.Orphans, c.Drift)
	}

	if len(rep.FRKeyDrift) > 0 {
		fmt.Fprintf(&b, "fr_key:    %d stale fr_key(s) — run `cusp doctor --fix`\n", len(rep.FRKeyDrift))
		for _, d := range rep.FRKeyDrift {
			fmt.Fprintf(&b, "  %s → %s\n", d.Stored, d.Derived)
		}
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
		b.WriteString("✗ problems found (see above)\n")
	}
	return b.String()
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "apply auto-fixable drift: migrate a behind database, recompute stale fr_keys")
	rootCmd.AddCommand(doctorCmd)
}
