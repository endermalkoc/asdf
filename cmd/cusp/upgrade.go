package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/selfupdate"
)

var flagUpgradeCheck bool

var upgradeCmd = &cobra.Command{
	Use:   "upgrade [version]",
	Short: "Download and install the latest cusp release in place",
	Long: `Self-update cusp.

Resolves the latest GitHub release (or the given [version] tag, e.g. v0.1.0),
downloads the archive for this OS/arch, verifies its SHA-256 against the release's
checksums.txt, and atomically replaces the running binary.

Use --check to report whether a newer release is available without installing it.
Requires write access to the directory the binary lives in (use sudo, or pin
CUSP_INSTALL_DIR via the install script, if it sits in a system path).

Examples:
  cusp upgrade
  cusp upgrade --check
  cusp upgrade v0.1.0
  cusp upgrade --json`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		target := ""
		if len(args) == 1 {
			target = args[0]
		}

		// --check (or the global --dry-run) reports availability without touching disk.
		if flagUpgradeCheck || flagDryRun {
			resolved := target
			if resolved == "" {
				v, err := selfupdate.LatestVersion(ctx, nil, selfupdate.DefaultRepo)
				if err != nil {
					return err
				}
				resolved = v
			}
			upToDate := selfupdate.SameVersion(version, resolved)
			info := map[string]any{
				"current":    version,
				"latest":     resolved,
				"up_to_date": upToDate,
			}
			if upToDate {
				emit(info, fmt.Sprintf("cusp %s is up to date.", version))
			} else {
				emit(info, fmt.Sprintf("A different release is available: %s (current: %s).\nRun 'cusp upgrade' to install it.", resolved, version))
			}
			return nil
		}

		progress := func(s string) {
			if !flagJSON {
				fmt.Fprintln(os.Stderr, s)
			}
		}
		res, err := selfupdate.Run(ctx, selfupdate.Options{
			Current: version,
			Target:  target,
		}, progress)
		if err != nil {
			return err
		}
		if res.UpToDate {
			emit(map[string]any{"upgraded": false, "version": version, "latest": res.Latest},
				fmt.Sprintf("cusp %s is already up to date.", version))
			return nil
		}
		emit(
			map[string]any{"upgraded": true, "previous": res.Previous, "version": res.Latest, "path": res.Path},
			fmt.Sprintf("Upgraded cusp %s → %s\n  %s\nRun 'cusp version' to confirm.", res.Previous, res.Latest, res.Path),
		)
		return nil
	},
}

func init() {
	upgradeCmd.Flags().BoolVar(&flagUpgradeCheck, "check", false, "report whether a newer release is available, without installing")
	rootCmd.AddCommand(upgradeCmd)
}
