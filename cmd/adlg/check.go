package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/asdf/internal/app"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate cross-reference and edge-graph integrity",
	Long: "Scan the whole graph for integrity problems: inline [[TYPE:key]] references that do\n" +
		"not resolve to an existing entity (e.g. a target was deleted or mistyped), and cycles\n" +
		"among the acyclic edge kinds. Read-only; honors the active changeset / --changeset.\n" +
		"Exits nonzero when any issue is found, so it can gate CI or an agent step.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		r, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		findings, err := app.Check(ctx, r)
		if err != nil {
			return err
		}
		if flagJSON {
			if findings == nil {
				findings = []app.CheckFinding{}
			}
			emit(findings, "")
		} else if len(findings) == 0 {
			fmt.Println("no integrity issues")
		} else {
			var b strings.Builder
			for _, f := range findings {
				fmt.Fprintf(&b, "  [%s] %s: %s\n", f.Kind, f.Location, f.Detail)
			}
			fmt.Print(b.String())
		}
		if len(findings) > 0 {
			return fmt.Errorf("%d integrity issue(s) found", len(findings))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)
}
