package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/asdf/internal/app"
)

var impactTransitive bool

var impactCmd = &cobra.Command{
	Use:   "impact <ref>",
	Short: "Show what references / depends on an entity (and what it relies on)",
	Long: "Traverse the cross-reference graph around an entity. <ref> is a TYPE:key reference\n" +
		"(e.g. REQ:ATT-FR-012, SPEC:ADDS, ENTITY:Student); a bare value is a requirement fr_key.\n" +
		"Reports inbound relationships (what references or points an edge at it — i.e. what's\n" +
		"affected if it changes) and outbound (what it references / relies on). --transitive adds\n" +
		"the reverse-edge closure (the full blast radius). Read-only; honors the active changeset.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		r, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		resolver, err := app.LoadResolver(ctx, r)
		if err != nil {
			return err
		}
		subject, ok := app.ResolveRef(resolver, args[0])
		if !ok {
			return app.NotFoundErr(fmt.Errorf("unknown entity %q — use TYPE:key (e.g. REQ:ATT-FR-012, SPEC:ADDS, ENTITY:Student)", args[0]))
		}
		rep, err := app.Impact(ctx, r, subject, impactTransitive)
		if err != nil {
			return err
		}
		if flagJSON {
			emit(rep, "")
			return nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "impact of %s\n", rep.Subject)
		writeLinks(&b, "referenced by / depended on (inbound)", rep.Inbound)
		writeLinks(&b, "references / depends on (outbound)", rep.Outbound)
		if impactTransitive {
			fmt.Fprintf(&b, "\ntransitive dependents via edges (%d):\n", len(rep.Transitive))
			if len(rep.Transitive) == 0 {
				b.WriteString("  (none)\n")
			}
			for _, t := range rep.Transitive {
				fmt.Fprintf(&b, "  %s\n", t)
			}
		}
		fmt.Print(b.String())
		return nil
	},
}

func writeLinks(b *strings.Builder, heading string, links []app.ImpactLink) {
	fmt.Fprintf(b, "\n%s (%d):\n", heading, len(links))
	if len(links) == 0 {
		b.WriteString("  (none)\n")
		return
	}
	for _, l := range links {
		fmt.Fprintf(b, "  %-28s [%s]\n", l.Endpoint, l.Via)
	}
}

func init() {
	impactCmd.Flags().BoolVar(&impactTransitive, "transitive", false, "also list the reverse-edge closure (full blast radius)")
	rootCmd.AddCommand(impactCmd)
}
