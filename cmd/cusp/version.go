package main

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// Build metadata. Overridden at release time via the linker, e.g.
//
//	-ldflags "-X main.version=v0.1.0 -X main.commit=abc1234 -X main.date=2026-06-24T00:00:00Z"
//
// (the Makefile and .goreleaser.yaml both set these). For a plain
// `go install …/cmd/cusp@v0.1.0` build the flags are absent, so init() below
// recovers the version and VCS stamp from the embedded module build info.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func init() {
	if version == "dev" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			if v := bi.Main.Version; v != "" && v != "(devel)" {
				version = v
			}
			for _, s := range bi.Settings {
				switch s.Key {
				case "vcs.revision":
					if commit == "" {
						commit = s.Value
					}
				case "vcs.time":
					if date == "" {
						date = s.Value
					}
				}
			}
		}
	}
	if len(commit) > 12 {
		commit = commit[:12]
	}
	rootCmd.Version = version
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version, commit, and build date",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		c, d := commit, date
		if c == "" {
			c = "none"
		}
		if d == "" {
			d = "unknown"
		}
		info := map[string]string{
			"version": version,
			"commit":  c,
			"date":    d,
			"go":      runtime.Version(),
			"os":      runtime.GOOS,
			"arch":    runtime.GOARCH,
		}
		human := fmt.Sprintf("cusp %s\n  commit: %s\n  built:  %s\n  go:     %s %s/%s",
			version, c, d, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		emit(info, human)
	},
}
