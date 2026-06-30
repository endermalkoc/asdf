package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/enums"
	"github.com/endermalkoc/cusp/internal/store"
)

var (
	domainDescription string
	domainName        string
	domainStatus      string
)

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
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: fmt.Sprintf("add domain %s", args[0]),
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

var domainShowCmd = &cobra.Command{
	Use:   "show <slug>",
	Short: "Show a domain's fields",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		rd, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		d, ok, err := store.GetDomain(ctx, rd, args[0])
		if err != nil {
			return err
		}
		if !ok {
			return app.NotFound("domain", args[0])
		}
		if flagJSON {
			emit(d, "")
			return nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s  (%s)\n", d.Slug, d.Status)
		fmt.Fprintf(&b, "  name:        %s\n", d.Name)
		if d.Description != "" {
			fmt.Fprintf(&b, "  description: %s\n", d.Description)
		}
		fmt.Fprintf(&b, "  id:          %s\n", d.ID)
		fmt.Print(b.String())
		return nil
	},
}

var domainEditCmd = &cobra.Command{
	Use:   "edit <slug>",
	Short: "Edit a domain's name/description/status (only the flags you pass change)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		if cmd.Flags().NFlag() == 0 {
			return fmt.Errorf("nothing to edit — pass --name/--description/--status")
		}
		var d store.Domain
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: "edit domain " + args[0],
			Validate: func(vctx context.Context, r store.Execer) error {
				cur, ok, e := store.GetDomain(vctx, r, args[0])
				if e != nil {
					return e
				}
				if !ok {
					return app.NotFound("domain", args[0])
				}
				if cmd.Flags().Changed("status") {
					if e := app.ValidateEnum("status", domainStatus, enums.DomainStatus); e != nil {
						return e
					}
					cur.Status = domainStatus
				}
				if cmd.Flags().Changed("name") {
					cur.Name = domainName
				}
				if cmd.Flags().Changed("description") {
					cur.Description = domainDescription
				}
				d = cur
				return nil
			},
		}, func(ctx context.Context, w *app.Write) error {
			if e := store.UpdateDomain(ctx, w.Tx, d.ID, d.Name, d.Description, d.Status); e != nil {
				return e
			}
			w.MarkDirty("req_domain")
			return nil
		})
		if err != nil {
			return err
		}
		emit(d, fmt.Sprintf("updated domain %s", d.Slug))
		return nil
	},
}

var domainDeleteCmd = &cobra.Command{
	Use:   "delete <slug>",
	Short: "Delete a domain and every spec under it (requirements, sections, stories, refs)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		var deleted store.Domain
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: "delete domain " + args[0],
		}, func(ctx context.Context, w *app.Write) error {
			d, ok, e := store.DeleteDomain(ctx, w.Tx, args[0])
			if e != nil {
				return e
			}
			if !ok {
				return app.NotFound("domain", args[0])
			}
			deleted = d
			for _, t := range []string{
				"req_domain", "req_spec", "req_requirement", "req_requirement_group", "req_user_story",
				"req_acceptance_scenario", "req_spec_section", "req_entity_ref", "req_edge", "pub_external_ref",
			} {
				w.MarkDirty(t)
			}
			return nil
		})
		if err != nil {
			return err
		}
		emit(deleted, fmt.Sprintf("deleted domain %s", deleted.Slug))
		return nil
	},
}

func init() {
	domainAddCmd.Flags().StringVar(&domainDescription, "description", "", "one-line domain description")
	domainEditCmd.Flags().StringVar(&domainName, "name", "", "new name")
	domainEditCmd.Flags().StringVar(&domainDescription, "description", "", "new description")
	domainEditCmd.Flags().StringVar(&domainStatus, "status", "", "status (draft|active|deprecated)")
	domainCmd.AddCommand(domainAddCmd, domainLsCmd, domainShowCmd, domainEditCmd, domainDeleteCmd)
	rootCmd.AddCommand(domainCmd)
}
