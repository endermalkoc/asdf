package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/ids"
	"github.com/endermalkoc/cusp/internal/store"
)

var (
	viewDomain string
	viewRoute  string
	viewSpec   string // spec slug (soft FK)
	viewTitle  string // edit
)

var viewCmd = &cobra.Command{Use: "view", Short: "Manage planning views (UI surfaces / routes)"}

// resolveViewSpec resolves an optional spec slug to a spec id, leaving it null when the slug is
// empty or does not uniquely match (a soft FK — the view stays usable without a backing spec).
func resolveViewSpec(ctx context.Context, r store.Execer, slug string) (string, error) {
	if slug == "" {
		return "", nil
	}
	id, ok, err := store.FindSpecIDBySlug(ctx, r, slug)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", app.NotFoundErr(fmt.Errorf("no unique spec with slug %q", slug))
	}
	return id, nil
}

var viewAddCmd = &cobra.Command{
	Use:   "add <title>",
	Short: "Add a planning view under a domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		title := args[0]
		id := ids.New()
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: "add view " + title,
			Validate: func(vctx context.Context, r store.Execer) error {
				if _, ok, e := store.DomainIDBySlug(vctx, r, viewDomain); e != nil {
					return e
				} else if !ok {
					return app.NotFoundErr(fmt.Errorf("unknown domain %q", viewDomain))
				}
				_, e := resolveViewSpec(vctx, r, viewSpec)
				return e
			},
		}, func(ctx context.Context, w *app.Write) error {
			domainID, _, e := store.DomainIDBySlug(ctx, w.Tx, viewDomain)
			if e != nil {
				return e
			}
			specID, e := resolveViewSpec(ctx, w.Tx, viewSpec)
			if e != nil {
				return e
			}
			if _, e := store.UpsertView(ctx, w.Tx, store.View{ID: id, Title: title, Route: viewRoute, SpecID: specID, DomainID: domainID}); e != nil {
				return e
			}
			w.MarkDirty("plan_view")
			return nil
		})
		if err != nil {
			return err
		}
		emit(map[string]any{"id": id, "title": title, "domain": viewDomain}, fmt.Sprintf("added view %q  (id=%s)", title, id))
		return nil
	},
}

var viewLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List planning views",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		r, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		vs, err := store.ListPlanViews(ctx, r)
		if err != nil {
			return err
		}
		if flagJSON {
			emit(vs, "")
			return nil
		}
		var b strings.Builder
		for _, v := range vs {
			fmt.Fprintf(&b, "%-26s %-12s %-28s %s\n", v.ID, v.DomainSlug, v.Route, v.Title)
		}
		fmt.Print(b.String())
		return nil
	},
}

var viewShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a view's fields",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		r, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		v, ok, err := store.GetPlanView(ctx, r, args[0])
		if err != nil {
			return err
		}
		if !ok {
			return app.NotFound("view", args[0])
		}
		if flagJSON {
			emit(v, "")
			return nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s\n", v.Title)
		fmt.Fprintf(&b, "  id:     %s\n", v.ID)
		fmt.Fprintf(&b, "  domain: %s\n", v.DomainSlug)
		if v.Route != "" {
			fmt.Fprintf(&b, "  route:  %s\n", v.Route)
		}
		if v.SpecSlug != "" {
			fmt.Fprintf(&b, "  spec:   %s (%s)\n", v.SpecSlug, v.SpecTitle)
		}
		fmt.Print(b.String())
		return nil
	},
}

var viewEditCmd = &cobra.Command{
	Use:   "edit <id>",
	Short: "Edit a view's title/route/domain/spec (only the flags you pass change)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		if cmd.Flags().NFlag() == 0 {
			return fmt.Errorf("nothing to edit — pass --title/--route/--domain/--spec")
		}
		id := args[0]
		var v store.PlanViewRow
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: "edit view " + id,
			Validate: func(vctx context.Context, r store.Execer) error {
				cur, ok, e := store.GetPlanView(vctx, r, id)
				if e != nil {
					return e
				}
				if !ok {
					return app.NotFound("view", id)
				}
				if cmd.Flags().Changed("title") {
					cur.Title = viewTitle
				}
				if cmd.Flags().Changed("route") {
					cur.Route = viewRoute
				}
				if cmd.Flags().Changed("domain") {
					if _, ok, e := store.DomainIDBySlug(vctx, r, viewDomain); e != nil {
						return e
					} else if !ok {
						return app.NotFoundErr(fmt.Errorf("unknown domain %q", viewDomain))
					}
					cur.DomainSlug = viewDomain
				}
				if cmd.Flags().Changed("spec") {
					if _, e := resolveViewSpec(vctx, r, viewSpec); e != nil {
						return e
					}
					cur.SpecSlug = viewSpec
				}
				v = cur
				return nil
			},
		}, func(ctx context.Context, w *app.Write) error {
			domainID, _, e := store.DomainIDBySlug(ctx, w.Tx, v.DomainSlug)
			if e != nil {
				return e
			}
			specID, e := resolveViewSpec(ctx, w.Tx, v.SpecSlug)
			if e != nil {
				return e
			}
			if _, e := store.UpsertView(ctx, w.Tx, store.View{ID: v.ID, Title: v.Title, Route: v.Route, SpecID: specID, DomainID: domainID}); e != nil {
				return e
			}
			w.MarkDirty("plan_view")
			return nil
		})
		if err != nil {
			return err
		}
		emit(v, fmt.Sprintf("updated view %s", id))
		return nil
	},
}

var viewDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a planning view (its deliverable links cascade)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: "delete view " + args[0],
		}, func(ctx context.Context, w *app.Write) error {
			ok, e := store.DeletePlanView(ctx, w.Tx, args[0])
			if e != nil {
				return e
			}
			if !ok {
				return app.NotFound("view", args[0])
			}
			for _, t := range []string{"plan_view", "plan_deliverable_view", "pub_external_ref", "req_edge", "req_entity_ref"} {
				w.MarkDirty(t)
			}
			return nil
		})
		if err != nil {
			return err
		}
		emit(map[string]any{"id": args[0]}, fmt.Sprintf("deleted view %s", args[0]))
		return nil
	},
}

func init() {
	viewAddCmd.Flags().StringVar(&viewDomain, "domain", "", "owning domain slug (required)")
	viewAddCmd.Flags().StringVar(&viewRoute, "route", "", "app route, e.g. /students/[id]")
	viewAddCmd.Flags().StringVar(&viewSpec, "spec", "", "backing spec slug (optional)")
	_ = viewAddCmd.MarkFlagRequired("domain")
	viewEditCmd.Flags().StringVar(&viewTitle, "title", "", "new title")
	viewEditCmd.Flags().StringVar(&viewRoute, "route", "", "app route")
	viewEditCmd.Flags().StringVar(&viewDomain, "domain", "", "owning domain slug")
	viewEditCmd.Flags().StringVar(&viewSpec, "spec", "", "backing spec slug")
	viewCmd.AddCommand(viewAddCmd, viewLsCmd, viewShowCmd, viewEditCmd, viewDeleteCmd)
	rootCmd.AddCommand(viewCmd)
}
