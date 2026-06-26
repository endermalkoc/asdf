package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/asdf/internal/app"
	"github.com/endermalkoc/asdf/internal/store"
)

var (
	specDomain string
	specPrefix string
	specTitle  string
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
		err = app.Mutate(ctx, ws, app.MutateOpts{
			Summary:   fmt.Sprintf("add spec %s", args[0]),
			Changeset: flagChangeset,
			Actor:     flagActor,
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

func init() {
	specAddCmd.Flags().StringVar(&specDomain, "domain", "", "owning domain slug (required)")
	specAddCmd.Flags().StringVar(&specPrefix, "prefix", "", "FR prefix, e.g. ATT (omit for FR-exempt docs)")
	specAddCmd.Flags().StringVar(&specTitle, "title", "", "spec title")
	_ = specAddCmd.MarkFlagRequired("domain")
	specCmd.AddCommand(specAddCmd)
	rootCmd.AddCommand(specCmd)
}
