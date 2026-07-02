package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/enums"
	"github.com/endermalkoc/cusp/internal/ids"
	"github.com/endermalkoc/cusp/internal/store"
)

var (
	delivSize      string
	delivStatus    string
	delivAIReady   string
	delivMilestone string
	delivTitle     string // edit
	delivView      string // link/unlink
	delivBlockedBy string // link/unlink
)

var deliverableCmd = &cobra.Command{Use: "deliverable", Aliases: []string{"deliv"}, Short: "Manage deliverables (planning)"}

// validateDeliverableEnums checks the size/status/ai_ready enums (each optional).
func validateDeliverableEnums() error {
	if e := app.ValidateEnum("size", delivSize, enums.DeliverableSize); e != nil {
		return e
	}
	if e := app.ValidateEnum("status", delivStatus, enums.DeliverableStatus); e != nil {
		return e
	}
	return app.ValidateEnum("ai-ready", delivAIReady, enums.DeliverableAIReady)
}

func resolveMilestone(ctx context.Context, r store.Execer, slug string) (string, error) {
	if slug == "" {
		return "", nil
	}
	id, ok, err := store.MilestoneIDBySlug(ctx, r, slug)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", app.NotFound("milestone", slug)
	}
	return id, nil
}

var deliverableAddCmd = &cobra.Command{
	Use:   "add <title>",
	Short: "Add a deliverable",
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
			Summary: "add deliverable " + title,
			Validate: func(vctx context.Context, r store.Execer) error {
				if e := validateDeliverableEnums(); e != nil {
					return e
				}
				_, e := resolveMilestone(vctx, r, delivMilestone)
				return e
			},
		}, func(ctx context.Context, w *app.Write) error {
			mid, e := resolveMilestone(ctx, w.Tx, delivMilestone)
			if e != nil {
				return e
			}
			if _, e := store.UpsertDeliverable(ctx, w.Tx, store.Deliverable{
				ID: id, Title: title, Size: delivSize, Status: delivStatus, AIReady: delivAIReady, MilestoneID: mid,
			}); e != nil {
				return e
			}
			w.MarkDirty("plan_deliverable")
			return nil
		})
		if err != nil {
			return err
		}
		emit(map[string]any{"id": id, "title": title}, fmt.Sprintf("added deliverable %q  (id=%s)", title, id))
		return nil
	},
}

var deliverableLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List deliverables",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		r, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		ds, err := store.ListDeliverables(ctx, r)
		if err != nil {
			return err
		}
		if flagJSON {
			emit(ds, "")
			return nil
		}
		var b strings.Builder
		for _, d := range ds {
			fmt.Fprintf(&b, "%-26s %-9s %-6s %-8s %s\n", d.ID, d.Status, d.Size, d.MilestoneSlug, d.Title)
		}
		fmt.Print(b.String())
		return nil
	},
}

var deliverableShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a deliverable's fields",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		r, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		d, ok, err := store.GetDeliverable(ctx, r, args[0])
		if err != nil {
			return err
		}
		if !ok {
			return app.NotFound("deliverable", args[0])
		}
		if flagJSON {
			emit(d, "")
			return nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s\n", d.Title)
		fmt.Fprintf(&b, "  id:        %s\n", d.ID)
		fmt.Fprintf(&b, "  status:    %s\n", d.Status)
		fmt.Fprintf(&b, "  size:      %s\n", d.Size)
		fmt.Fprintf(&b, "  ai_ready:  %s\n", d.AIReady)
		fmt.Fprintf(&b, "  milestone: %s\n", d.MilestoneSlug)
		fmt.Print(b.String())
		return nil
	},
}

var deliverableEditCmd = &cobra.Command{
	Use:   "edit <id>",
	Short: "Edit a deliverable's title/size/status/ai-ready/milestone (only the flags you pass change)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		if cmd.Flags().NFlag() == 0 {
			return fmt.Errorf("nothing to edit — pass --title/--size/--status/--ai-ready/--milestone")
		}
		id := args[0]
		var d store.DeliverableRow
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: "edit deliverable " + id,
			Validate: func(vctx context.Context, r store.Execer) error {
				cur, ok, e := store.GetDeliverable(vctx, r, id)
				if e != nil {
					return e
				}
				if !ok {
					return app.NotFound("deliverable", id)
				}
				if cmd.Flags().Changed("title") {
					cur.Title = delivTitle
				}
				if cmd.Flags().Changed("size") {
					if e := app.ValidateEnum("size", delivSize, enums.DeliverableSize); e != nil {
						return e
					}
					cur.Size = delivSize
				}
				if cmd.Flags().Changed("status") {
					if e := app.ValidateEnum("status", delivStatus, enums.DeliverableStatus); e != nil {
						return e
					}
					cur.Status = delivStatus
				}
				if cmd.Flags().Changed("ai-ready") {
					if e := app.ValidateEnum("ai-ready", delivAIReady, enums.DeliverableAIReady); e != nil {
						return e
					}
					cur.AIReady = delivAIReady
				}
				if cmd.Flags().Changed("milestone") {
					if _, e := resolveMilestone(vctx, r, delivMilestone); e != nil {
						return e
					}
					cur.MilestoneSlug = delivMilestone
				}
				d = cur
				return nil
			},
		}, func(ctx context.Context, w *app.Write) error {
			mid, e := resolveMilestone(ctx, w.Tx, d.MilestoneSlug)
			if e != nil {
				return e
			}
			if _, e := store.UpsertDeliverable(ctx, w.Tx, store.Deliverable{
				ID: d.ID, Title: d.Title, Size: d.Size, Status: d.Status, AIReady: d.AIReady, MilestoneID: mid,
			}); e != nil {
				return e
			}
			w.MarkDirty("plan_deliverable")
			return nil
		})
		if err != nil {
			return err
		}
		emit(d, fmt.Sprintf("updated deliverable %s", id))
		return nil
	},
}

var deliverableDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a deliverable (its links cascade)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: "delete deliverable " + args[0],
		}, func(ctx context.Context, w *app.Write) error {
			ok, e := store.DeleteDeliverable(ctx, w.Tx, args[0])
			if e != nil {
				return e
			}
			if !ok {
				return app.NotFound("deliverable", args[0])
			}
			for _, t := range []string{"plan_deliverable", "plan_capability_deliverable", "plan_deliverable_view", "plan_deliverable_dependency", "pub_external_ref", "req_edge", "req_entity_ref"} {
				w.MarkDirty(t)
			}
			return nil
		})
		if err != nil {
			return err
		}
		emit(map[string]any{"id": args[0]}, fmt.Sprintf("deleted deliverable %s", args[0]))
		return nil
	},
}

var deliverableLinkCmd = &cobra.Command{
	Use:   "link <id>",
	Short: "Link a deliverable to a view (--view) or a blocker (--blocked-by)",
	Args:  cobra.ExactArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return deliverableLink(cmd, args[0], false) },
}

var deliverableUnlinkCmd = &cobra.Command{
	Use:   "unlink <id>",
	Short: "Unlink a deliverable from a view (--view) or a blocker (--blocked-by)",
	Args:  cobra.ExactArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return deliverableLink(cmd, args[0], true) },
}

func deliverableLink(cmd *cobra.Command, id string, unlink bool) error {
	ctx := cmd.Context()
	if delivView == "" && delivBlockedBy == "" {
		return app.ValidationFailed(fmt.Errorf("pass --view <id> or --blocked-by <deliverable-id>"))
	}
	ws, err := connect(ctx)
	if err != nil {
		return err
	}
	defer ws.Close()
	verb := "link"
	if unlink {
		verb = "unlink"
	}
	err = runMutate(cmd, ws, app.MutateOpts{
		Summary: verb + " deliverable " + id,
		Validate: func(vctx context.Context, r store.Execer) error {
			if _, ok, e := store.GetDeliverable(vctx, r, id); e != nil {
				return e
			} else if !ok {
				return app.NotFound("deliverable", id)
			}
			return nil
		},
	}, func(ctx context.Context, w *app.Write) error {
		if delivView != "" {
			if _, ok, e := store.GetPlanView(ctx, w.Tx, delivView); e != nil {
				return e
			} else if !ok {
				return app.NotFound("view", delivView)
			}
			var e error
			if unlink {
				e = store.UnlinkDeliverableView(ctx, w.Tx, id, delivView)
			} else {
				e = store.LinkDeliverableView(ctx, w.Tx, id, delivView)
			}
			if e != nil {
				return e
			}
			w.MarkDirty("plan_deliverable_view")
		}
		if delivBlockedBy != "" {
			if _, ok, e := store.GetDeliverable(ctx, w.Tx, delivBlockedBy); e != nil {
				return e
			} else if !ok {
				return app.NotFound("deliverable", delivBlockedBy)
			}
			var e error
			if unlink {
				e = store.UnlinkDeliverableDependency(ctx, w.Tx, id, delivBlockedBy)
			} else {
				e = store.LinkDeliverableDependency(ctx, w.Tx, id, delivBlockedBy)
			}
			if e != nil {
				return e
			}
			w.MarkDirty("plan_deliverable_dependency")
		}
		return nil
	})
	if err != nil {
		return err
	}
	emit(map[string]any{"deliverable": id, "view": delivView, "blockedBy": delivBlockedBy, "unlinked": unlink},
		fmt.Sprintf("%sed deliverable %s", verb, id))
	return nil
}

func init() {
	delivFlags := func(c *cobra.Command, forEdit bool) {
		c.Flags().StringVar(&delivSize, "size", "", "size (S|M|L|XL)")
		c.Flags().StringVar(&delivStatus, "status", "", "status (proposed|specced|wired|built|ship)")
		c.Flags().StringVar(&delivAIReady, "ai-ready", "", "AI-ready (yes|no|na)")
		c.Flags().StringVar(&delivMilestone, "milestone", "", "milestone slug")
		if forEdit {
			c.Flags().StringVar(&delivTitle, "title", "", "new title")
		}
	}
	delivFlags(deliverableAddCmd, false)
	delivFlags(deliverableEditCmd, true)
	for _, c := range []*cobra.Command{deliverableLinkCmd, deliverableUnlinkCmd} {
		c.Flags().StringVar(&delivView, "view", "", "view id")
		c.Flags().StringVar(&delivBlockedBy, "blocked-by", "", "blocking deliverable id")
	}
	deliverableCmd.AddCommand(deliverableAddCmd, deliverableLsCmd, deliverableShowCmd, deliverableEditCmd, deliverableDeleteCmd, deliverableLinkCmd, deliverableUnlinkCmd)
	rootCmd.AddCommand(deliverableCmd)
}
