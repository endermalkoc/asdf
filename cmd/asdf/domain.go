package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/asdf/internal/app"
	"github.com/endermalkoc/asdf/internal/enums"
	"github.com/endermalkoc/asdf/internal/store"
)

var (
	domainKind        string
	domainDescription string
)

var domainCmd = &cobra.Command{Use: "domain", Short: "Manage domains"}

var domainAddCmd = &cobra.Command{
	Use:   "add <abbreviation> <name>",
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
			Validate: func(context.Context) error {
				note, e := app.ValidateEnumSoft("domain kind", domainKind, enums.DomainKind, flagStrict)
				if note != "" {
					fmt.Fprintln(cmd.ErrOrStderr(), note)
				}
				return e
			},
		}, func(ctx context.Context, w *app.Write) error {
			res, e := store.AddDomain(ctx, w.Tx, store.Domain{Abbreviation: args[0], Name: args[1], Description: domainDescription, Kind: domainKind})
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
		emit(d, fmt.Sprintf("added domain %s — %s  (%s, id=%s)", d.Abbreviation, d.Name, d.Kind, d.ID))
		return nil
	},
}

var domainLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List domains",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		domains, err := store.ListDomains(ctx, ws.DB())
		if err != nil {
			return err
		}
		if flagJSON {
			emit(domains, "")
			return nil
		}
		var b strings.Builder
		for _, d := range domains {
			fmt.Fprintf(&b, "%-10s %-24s %-14s %s\n", d.Abbreviation, d.Name, d.Kind, d.Status)
		}
		fmt.Print(b.String())
		return nil
	},
}

func init() {
	domainAddCmd.Flags().StringVar(&domainKind, "kind", "service", "domain kind (service|shared|infrastructure|entities|analysis)")
	domainAddCmd.Flags().StringVar(&domainDescription, "description", "", "one-line domain description")
	domainCmd.AddCommand(domainAddCmd, domainLsCmd)
	rootCmd.AddCommand(domainCmd)
}
