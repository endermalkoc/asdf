package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/asdf/internal/app"
	"github.com/endermalkoc/asdf/internal/enums"
	"github.com/endermalkoc/asdf/internal/store"
)

var (
	specDomain string
	specPrefix string
	specTitle  string
	specStatus string
)

var specCmd = &cobra.Command{Use: "spec", Short: "Manage specs"}

var specAddCmd = &cobra.Command{
	Use:   "add <path>",
	Short: "Add a spec under a domain (path is domain-relative, e.g. add-student.md)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		var sp store.Spec
		var resolved app.ResolvedRefs
		var title string
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: fmt.Sprintf("add spec %s", args[0]),
			Validate: func(vctx context.Context, r store.Execer) error {
				if err := app.ValidateRequired("domain", specDomain); err != nil {
					return err
				}
				resolver, e := app.LoadResolver(vctx, r)
				if e != nil {
					return e
				}
				// Canonicalize + resolve inline refs through the shared ingestion path
				// (same as the importer), so the stored title carries canonical links.
				var rw []string
				rw, resolved = app.IngestRefs(resolver, "spec", "", specTitle)
				title = rw[0]
				if !flagForce {
					return app.DanglingError(resolved.Dangling)
				}
				return nil
			},
		}, func(ctx context.Context, w *app.Write) error {
			// The path arg is domain-relative and includes the filename (e.g.
			// student-detail/add.md); store the directory in path and the filename
			// stem in slug (the filename is reconstructed as slug.md).
			dir := filepath.Dir(args[0])
			if dir == "." {
				dir = ""
			}
			res, e := store.AddSpec(ctx, w.Tx, specDomain, store.Spec{
				Path:   dir,
				Slug:   strings.TrimSuffix(filepath.Base(args[0]), filepath.Ext(args[0])),
				Prefix: specPrefix,
				Title:  title,
			})
			if e != nil {
				return e
			}
			w.MarkDirty("req_spec")
			sp = res
			return app.ReconcileRefs(ctx, w, "spec", sp.ID, resolved.Targets)
		})
		if err != nil {
			return err
		}
		label := sp.Prefix
		if label == "" {
			label = "(no prefix)"
		}
		emit(sp, fmt.Sprintf("added spec %s — %s  (id=%s)", label, sp.Path, sp.ID))
		return nil
	},
}

var specShowCmd = &cobra.Command{
	Use:   "show <prefix-or-path>",
	Short: "Show a spec's fields",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		rd, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		sp, ok, err := store.GetSpec(ctx, rd, args[0])
		if err != nil {
			return err
		}
		if !ok {
			return app.NotFound("spec", args[0])
		}
		if flagJSON {
			emit(sp, "")
			return nil
		}
		label := sp.Prefix
		if label == "" {
			label = "(no prefix)"
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s  (%s)\n", label, sp.Status)
		if sp.Title != "" {
			fmt.Fprintf(&b, "  title:  %s\n", sp.Title)
		}
		fmt.Fprintf(&b, "  domain: %s\n", sp.DomainSlug)
		fmt.Fprintf(&b, "  path:   %s\n", store.SpecDocPath(sp.DomainSlug, sp.Path, sp.Slug))
		fmt.Fprintf(&b, "  id:     %s\n", sp.ID)
		fmt.Print(b.String())
		return nil
	},
}

var specEditCmd = &cobra.Command{
	Use:   "edit <prefix-or-path>",
	Short: "Edit a spec's title/status (only the flags you pass change)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		if cmd.Flags().NFlag() == 0 {
			return fmt.Errorf("nothing to edit — pass --title and/or --status")
		}
		var updated store.SpecRow
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: "edit spec " + args[0],
			Validate: func(vctx context.Context, r store.Execer) error {
				cur, ok, e := store.GetSpec(vctx, r, args[0])
				if e != nil {
					return e
				}
				if !ok {
					return app.NotFound("spec", args[0])
				}
				if cmd.Flags().Changed("status") {
					if e := app.ValidateEnum("status", specStatus, enums.SpecStatus); e != nil {
						return e
					}
					cur.Status = specStatus
				}
				if cmd.Flags().Changed("title") {
					cur.Title = specTitle
				}
				updated = cur
				return nil
			},
		}, func(ctx context.Context, w *app.Write) error {
			if e := store.UpdateSpec(ctx, w.Tx, updated.ID, updated.Title, updated.Status); e != nil {
				return e
			}
			w.MarkDirty("req_spec")
			return nil
		})
		if err != nil {
			return err
		}
		emit(updated, fmt.Sprintf("updated spec %s", args[0]))
		return nil
	},
}

var specDeleteCmd = &cobra.Command{
	Use:   "delete <prefix-or-path>",
	Short: "Delete a spec and everything under it (requirements, sections, stories, scenarios, refs)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		var deleted store.SpecRow
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: "delete spec " + args[0],
		}, func(ctx context.Context, w *app.Write) error {
			d, ok, e := store.DeleteSpec(ctx, w.Tx, args[0])
			if e != nil {
				return e
			}
			if !ok {
				return app.NotFound("spec", args[0])
			}
			deleted = d
			// The spec row + every table the FK cascade touches + the polymorphic ref
			// tables DeleteSpec cleaned — all must be staged for a clean working set.
			for _, t := range []string{
				"req_spec", "req_requirement", "req_requirement_group", "req_user_story",
				"req_acceptance_scenario", "req_spec_section", "req_entity_ref", "req_edge", "pub_external_ref",
			} {
				w.MarkDirty(t)
			}
			return nil
		})
		if err != nil {
			return err
		}
		label := deleted.Prefix
		if label == "" {
			label = deleted.Slug
		}
		emit(deleted, fmt.Sprintf("deleted spec %s", label))
		return nil
	},
}

func init() {
	specAddCmd.Flags().StringVar(&specDomain, "domain", "", "owning domain slug (required)")
	specAddCmd.Flags().StringVar(&specPrefix, "prefix", "", "FR prefix, e.g. ATT (omit for FR-exempt docs)")
	specAddCmd.Flags().StringVar(&specTitle, "title", "", "spec title")
	_ = specAddCmd.MarkFlagRequired("domain")

	specEditCmd.Flags().StringVar(&specTitle, "title", "", "new title")
	specEditCmd.Flags().StringVar(&specStatus, "status", "", "status (draft|reviewed|active|obsolete)")

	specCmd.AddCommand(specAddCmd, specShowCmd, specEditCmd, specDeleteCmd)
	rootCmd.AddCommand(specCmd)
}
