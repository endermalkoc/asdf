package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/asdf/internal/app"
	"github.com/endermalkoc/asdf/internal/store"
)

var domainDescription string

var domainCmd = &cobra.Command{Use: "domain", Short: "Manage domains"}

var domainAddCmd = &cobra.Command{
	Use:   "add <slug> <name>",
	Short: "Add a domain",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		var d store.Domain
		err = app.Mutate(ctx, ws, app.MutateOpts{
			Summary:   fmt.Sprintf("add domain %s", args[0]),
			Changeset: flagChangeset,
			Actor:     flagActor,
		}, func(ctx context.Context, w *app.Write) error {
			res, e := store.AddDomain(ctx, w.Tx, store.Domain{Slug: args[0], Name: args[1], Description: domainDescription})
			if e != nil {
				return e
			}
			w.MarkDirty("req_domain")
			d = res
			return nil
		})
		if err != nil {
			return err
		}
		emit(d, fmt.Sprintf("added domain %s — %s  (id=%s)", d.Slug, d.Name, d.ID))
		return nil
	},
}

var domainLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List domains",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		r, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		domains, err := store.ListDomains(ctx, r)
		if err != nil {
			return err
		}
		if flagJSON {
			emit(domains, "")
			return nil
		}
		var b strings.Builder
		for _, d := range domains {
			fmt.Fprintf(&b, "%-10s %-24s %s\n", d.Slug, d.Name, d.Status)
		}
		fmt.Print(b.String())
		return nil
	},
}

func init() {
	domainAddCmd.Flags().StringVar(&domainDescription, "description", "", "one-line domain description")
	domainCmd.AddCommand(domainAddCmd, domainLsCmd)
	rootCmd.AddCommand(domainCmd)
}
