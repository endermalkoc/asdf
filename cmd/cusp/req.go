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
	reqDelivery      string
	reqMilestone     string
	reqPriority      int
	reqStatement     string
	reqNotes         string
	reqContentStatus string
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
		var statement string
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: fmt.Sprintf("add requirement to %s", args[0]),
			Validate: func(vctx context.Context, r store.Execer) error {
				if note, e := app.ValidateEnumSoft("delivery status", reqDelivery, enums.RequirementDelivery, flagStrict); e != nil {
					return e
				} else if note != "" {
					fmt.Fprintln(cmd.ErrOrStderr(), note)
				}
				resolver, e := app.LoadResolver(vctx, r)
				if e != nil {
					return e
				}
				// Canonicalize bare FR mentions in the statement into [[REQ:..]] tokens and
				// resolve them — the same ingestion the importer runs, so a CLI-authored
				// requirement carries (and validates) the same links as an imported one.
				var rw []string
				rw, resolved = app.IngestRefs(resolver, "requirement", "", args[1])
				statement = rw[0]
				if cmd.Flags().Changed("priority") {
					if _, ok, e := store.PriorityByLevel(vctx, r, reqPriority); e != nil {
						return e
					} else if !ok {
						return fmt.Errorf("invalid priority %d (see `cusp priority ls`)", reqPriority)
					}
				}
				if !flagForce {
					return app.DanglingError(resolved.Dangling)
				}
				return nil
			},
		}, func(ctx context.Context, w *app.Write) error {
			var prio *int
			if cmd.Flags().Changed("priority") {
				prio = &reqPriority
			}
			res, e := store.AddRequirement(ctx, w.Tx, args[0], store.Requirement{
				Statement:      statement,
				DeliveryStatus: reqDelivery,
				MilestoneID:    reqMilestone,
				Priority:       prio,
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
		rd, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		reqs, err := store.ListRequirements(ctx, rd, args[0])
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

var reqShowCmd = &cobra.Command{
	Use:   "show <fr-key>",
	Short: "Show a requirement's fields",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		rd, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		req, ok, err := store.GetRequirement(ctx, rd, args[0])
		if err != nil {
			return err
		}
		if !ok {
			return app.NotFound("requirement", args[0])
		}
		if flagJSON {
			emit(req, "")
			return nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s  (%s)\n", req.FRKey, req.ContentStatus)
		fmt.Fprintf(&b, "  statement: %s\n", req.Statement)
		if req.DeliveryStatus != "" {
			fmt.Fprintf(&b, "  delivery:  %s\n", req.DeliveryStatus)
		}
		if req.Priority != nil {
			fmt.Fprintf(&b, "  priority:  %d\n", *req.Priority)
		}
		if req.Notes != "" {
			fmt.Fprintf(&b, "  notes:     %s\n", req.Notes)
		}
		fmt.Fprintf(&b, "  id:        %s\n", req.ID)
		fmt.Print(b.String())
		return nil
	},
}

var reqEditCmd = &cobra.Command{
	Use:   "edit <fr-key>",
	Short: "Edit a requirement (only the flags you pass change)",
	Long: "Edit a requirement's mutable fields. Only flags you pass are changed; identity\n" +
		"(number/fr_key) is fixed. Editing --statement re-canonicalizes its inline links and\n" +
		"re-validates + reconciles the requirement's cross-references (same as `req add`).",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		frKey := args[0]
		changed := func(n string) bool { return cmd.Flags().Changed(n) }
		if cmd.Flags().NFlag() == 0 {
			return fmt.Errorf("nothing to edit — pass at least one of --statement/--delivery/--milestone-id/--notes/--content-status/--priority")
		}
		var updated store.Requirement
		var resolved app.ResolvedRefs
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: "edit requirement " + frKey,
			Validate: func(vctx context.Context, r store.Execer) error {
				cur, ok, e := store.GetRequirement(vctx, r, frKey)
				if e != nil {
					return e
				}
				if !ok {
					return app.NotFound("requirement", frKey)
				}
				if changed("content-status") {
					if e := app.ValidateEnum("content status", reqContentStatus, enums.ContentStatus); e != nil {
						return e
					}
					cur.ContentStatus = reqContentStatus
				}
				if changed("delivery") {
					if note, e := app.ValidateEnumSoft("delivery status", reqDelivery, enums.RequirementDelivery, flagStrict); e != nil {
						return e
					} else if note != "" {
						fmt.Fprintln(cmd.ErrOrStderr(), note)
					}
					cur.DeliveryStatus = reqDelivery
				}
				if changed("milestone-id") {
					cur.MilestoneID = reqMilestone
				}
				if changed("notes") {
					cur.Notes = reqNotes
				}
				if changed("priority") {
					if _, ok, e := store.PriorityByLevel(vctx, r, reqPriority); e != nil {
						return e
					} else if !ok {
						return fmt.Errorf("invalid priority %d (see `cusp priority ls`)", reqPriority)
					}
					p := reqPriority
					cur.Priority = &p
				}
				if changed("statement") {
					resolver, e := app.LoadResolver(vctx, r)
					if e != nil {
						return e
					}
					rw, res := app.IngestRefs(resolver, "requirement", cur.ID, reqStatement)
					cur.Statement, resolved = rw[0], res
					if !flagForce {
						if e := app.DanglingError(res.Dangling); e != nil {
							return e
						}
					}
				}
				updated = cur
				return nil
			},
		}, func(ctx context.Context, w *app.Write) error {
			if e := store.UpdateRequirement(ctx, w.Tx, updated); e != nil {
				return e
			}
			w.MarkDirty("req_requirement")
			if changed("statement") {
				return app.ReconcileRefs(ctx, w, "requirement", updated.ID, resolved.Targets)
			}
			return nil
		})
		if err != nil {
			return err
		}
		emit(updated, fmt.Sprintf("updated %s", updated.FRKey))
		return nil
	},
}

var reqDeleteCmd = &cobra.Command{
	Use:   "delete <fr-key>",
	Short: "Delete a requirement and every reference to/from it",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		var deleted store.Requirement
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: "delete requirement " + args[0],
		}, func(ctx context.Context, w *app.Write) error {
			d, ok, e := store.DeleteRequirement(ctx, w.Tx, args[0])
			if e != nil {
				return e
			}
			if !ok {
				return app.NotFound("requirement", args[0])
			}
			deleted = d
			w.MarkDirty("req_requirement")
			w.MarkDirty("req_entity_ref")
			w.MarkDirty("req_edge")
			w.MarkDirty("pub_external_ref")
			return nil
		})
		if err != nil {
			return err
		}
		emit(deleted, fmt.Sprintf("deleted %s", deleted.FRKey))
		return nil
	},
}

// priorityCmd exposes the standard 0–4 priority taxonomy.
var priorityCmd = &cobra.Command{Use: "priority", Short: "The standard 0–4 priority levels"}

var priorityLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List the priority levels (0 = most urgent)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		r, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		ps, err := store.ListPriorities(ctx, r)
		if err != nil {
			return err
		}
		if flagJSON {
			emit(ps, "")
			return nil
		}
		var b strings.Builder
		for _, p := range ps {
			fmt.Fprintf(&b, "%d  %-9s %s\n", p.Level, p.Label, p.Description)
		}
		fmt.Print(b.String())
		return nil
	},
}

func init() {
	reqAddCmd.Flags().StringVar(&reqDelivery, "delivery", "", "delivery status (covered|test-pending|not-implemented|...)")
	reqAddCmd.Flags().StringVar(&reqMilestone, "milestone-id", "", "milestone id")
	reqAddCmd.Flags().IntVar(&reqPriority, "priority", 0, "priority level 0–4 (0=Critical … 4=Backlog; see `priority ls`)")

	reqEditCmd.Flags().StringVar(&reqStatement, "statement", "", "replace the statement text (re-validates inline links)")
	reqEditCmd.Flags().StringVar(&reqDelivery, "delivery", "", "delivery status (covered|test-pending|not-implemented|...)")
	reqEditCmd.Flags().StringVar(&reqMilestone, "milestone-id", "", "milestone id")
	reqEditCmd.Flags().StringVar(&reqNotes, "notes", "", "notes")
	reqEditCmd.Flags().StringVar(&reqContentStatus, "content-status", "", "content status (draft|active|obsolete)")
	reqEditCmd.Flags().IntVar(&reqPriority, "priority", 0, "priority level 0–4 (see `priority ls`)")

	reqCmd.AddCommand(reqAddCmd, reqLsCmd, reqShowCmd, reqEditCmd, reqDeleteCmd)
	priorityCmd.AddCommand(priorityLsCmd)
	rootCmd.AddCommand(reqCmd, priorityCmd)
}
