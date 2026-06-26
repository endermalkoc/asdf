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
	reqDelivery  string
	reqMilestone string
)

var reqCmd = &cobra.Command{Use: "req", Short: "Manage functional requirements"}

var reqAddCmd = &cobra.Command{
	Use:   "add <spec-prefix> <statement>",
	Short: "Add a requirement to a spec (auto-numbers and derives the FR key)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		var r store.Requirement
		var resolved app.ResolvedRefs
		err = app.Mutate(ctx, ws, app.MutateOpts{
			Summary:   fmt.Sprintf("add requirement to %s", args[0]),
			Changeset: flagChangeset,
			Actor:     flagActor,
			Validate: func(vctx context.Context) error {
				if note, e := app.ValidateEnumSoft("delivery status", reqDelivery, enums.RequirementDelivery, flagStrict); e != nil {
					return e
				} else if note != "" {
					fmt.Fprintln(cmd.ErrOrStderr(), note)
				}
				resolver, e := app.LoadResolver(vctx, ws.DB())
				if e != nil {
					return e
				}
				resolved = app.ScanRefs(resolver, "requirement", "", args[1])
				if !flagForce {
					return app.DanglingError(resolved.Dangling)
				}
				return nil
			},
		}, func(ctx context.Context, w *app.Write) error {
			res, e := store.AddRequirement(ctx, w.Tx, args[0], store.Requirement{
				Statement:      args[1],
				DeliveryStatus: reqDelivery,
				MilestoneID:    reqMilestone,
			})
			if e != nil {
				return e
			}
			w.MarkDirty("req_requirement")
			r = res
			return app.ReconcileRefs(ctx, w, "requirement", r.ID, resolved.Targets)
		})
		if err != nil {
			return err
		}
		emit(r, fmt.Sprintf("added %s — %s  (id=%s)", r.FRKey, r.Statement, r.ID))
		return nil
	},
}

var reqLsCmd = &cobra.Command{
	Use:   "ls <spec-prefix>",
	Short: "List a spec's requirements",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		reqs, err := store.ListRequirements(ctx, ws.DB(), args[0])
		if err != nil {
			return err
		}
		if flagJSON {
			emit(reqs, "")
			return nil
		}
		var b strings.Builder
		for _, r := range reqs {
			ds := r.DeliveryStatus
			if ds == "" {
				ds = "-"
			}
			fmt.Fprintf(&b, "%-12s [%-14s] %s\n", r.FRKey, ds, r.Statement)
		}
		fmt.Print(b.String())
		return nil
	},
}

func init() {
	reqAddCmd.Flags().StringVar(&reqDelivery, "delivery", "", "delivery status (covered|test-pending|not-implemented|...)")
	reqAddCmd.Flags().StringVar(&reqMilestone, "milestone-id", "", "milestone id")
	reqCmd.AddCommand(reqAddCmd, reqLsCmd)
	rootCmd.AddCommand(reqCmd)
}
