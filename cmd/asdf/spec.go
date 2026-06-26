package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/asdf/internal/app"
	"github.com/endermalkoc/asdf/internal/enums"
	"github.com/endermalkoc/asdf/internal/store"
)

var (
	specDomain string
	specPrefix string
	specTitle  string
	specKind   string
)

var specCmd = &cobra.Command{Use: "spec", Short: "Manage specs"}

var specAddCmd = &cobra.Command{
	Use:   "add <path>",
	Short: "Add a spec document under a domain",
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
		err = app.Mutate(ctx, ws, app.MutateOpts{
			Summary:   fmt.Sprintf("add spec %s", args[0]),
			Changeset: flagChangeset,
			Actor:     flagActor,
			Validate: func(vctx context.Context) error {
				if err := app.ValidateRequired("domain", specDomain); err != nil {
					return err
				}
				if note, err := app.ValidateEnumSoft("spec kind", specKind, enums.SpecKind, flagStrict); err != nil {
					return err
				} else if note != "" {
					fmt.Fprintln(cmd.ErrOrStderr(), note)
				}
				resolver, e := app.LoadResolver(vctx, ws.DB())
				if e != nil {
					return e
				}
				resolved = app.ScanRefs(resolver, "spec", "", specTitle)
				if !flagForce {
					return app.DanglingError(resolved.Dangling)
				}
				return nil
			},
		}, func(ctx context.Context, w *app.Write) error {
			res, e := store.AddSpec(ctx, w.Tx, specDomain, store.Spec{
				Path:   args[0],
				Prefix: specPrefix,
				Title:  specTitle,
				Kind:   specKind,
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
	specAddCmd.Flags().StringVar(&specDomain, "domain", "", "owning domain abbreviation (required)")
	specAddCmd.Flags().StringVar(&specPrefix, "prefix", "", "FR prefix, e.g. ATT (omit for FR-exempt docs)")
	specAddCmd.Flags().StringVar(&specTitle, "title", "", "spec title")
	specAddCmd.Flags().StringVar(&specKind, "kind", "feature", "spec kind (feature|entity|journey|analysis|index|meta|reference)")
	_ = specAddCmd.MarkFlagRequired("domain")
	specCmd.AddCommand(specAddCmd)
	rootCmd.AddCommand(specCmd)
}
