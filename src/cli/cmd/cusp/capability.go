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
	capLevel       string
	capDomain      string
	capParent      string
	capMilestone   string // link/unlink target
	capDeliverable string // link/unlink target
	capTitle       string // edit
)

var capabilityCmd = &cobra.Command{Use: "capability", Aliases: []string{"cap"}, Short: "Manage capabilities (planning)"}

var capabilityAddCmd = &cobra.Command{
	Use:   "add <title>",
	Short: "Add a capability under a domain",
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
			Summary: "add capability " + title,
			Validate: func(vctx context.Context, r store.Execer) error {
				if e := app.ValidateEnum("level", capLevel, enums.CapabilityLevel); e != nil {
					return e
				}
				if _, ok, e := store.DomainIDBySlug(vctx, r, capDomain); e != nil {
					return e
				} else if !ok {
					return app.NotFoundErr(fmt.Errorf("unknown domain %q", capDomain))
				}
				if capParent != "" {
					if _, ok, e := store.GetCapability(vctx, r, capParent); e != nil {
						return e
					} else if !ok {
						return app.NotFound("parent capability", capParent)
					}
				}
				return nil
			},
		}, func(ctx context.Context, w *app.Write) error {
			domainID, _, e := store.DomainIDBySlug(ctx, w.Tx, capDomain)
			if e != nil {
				return e
			}
			if _, e := store.UpsertCapability(ctx, w.Tx, store.Capability{ID: id, Title: title, Level: capLevel, DomainID: domainID}); e != nil {
				return e
			}
			if capParent != "" {
				if e := store.SetCapabilityParent(ctx, w.Tx, id, capParent); e != nil {
					return e
				}
			}
			w.MarkDirty("plan_capability")
			return nil
		})
		if err != nil {
			return err
		}
		emit(map[string]any{"id": id, "title": title, "level": capLevel, "domain": capDomain},
			fmt.Sprintf("added capability %q  (id=%s)", title, id))
		return nil
	},
}

var capabilityLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List capabilities",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		r, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		caps, err := store.ListCapabilities(ctx, r)
		if err != nil {
			return err
		}
		if flagJSON {
			emit(caps, "")
			return nil
		}
		var b strings.Builder
		for _, c := range caps {
			fmt.Fprintf(&b, "%-26s %-11s %-12s %s\n", c.ID, c.Level, c.DomainSlug, c.Title)
		}
		fmt.Print(b.String())
		return nil
	},
}

var capabilityShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a capability's fields",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		r, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		c, ok, err := store.GetCapability(ctx, r, args[0])
		if err != nil {
			return err
		}
		if !ok {
			return app.NotFound("capability", args[0])
		}
		if flagJSON {
			emit(c, "")
			return nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s\n", c.Title)
		fmt.Fprintf(&b, "  id:      %s\n", c.ID)
		fmt.Fprintf(&b, "  level:   %s\n", c.Level)
		fmt.Fprintf(&b, "  domain:  %s\n", c.DomainSlug)
		if c.ParentID != "" {
			fmt.Fprintf(&b, "  parent:  %s\n", c.ParentID)
		}
		fmt.Print(b.String())
		return nil
	},
}

var capabilityEditCmd = &cobra.Command{
	Use:   "edit <id>",
	Short: "Edit a capability's title/level/domain/parent (only the flags you pass change)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		if cmd.Flags().NFlag() == 0 {
			return fmt.Errorf("nothing to edit — pass --title/--level/--domain/--parent")
		}
		id := args[0]
		var capRow store.CapabilityRow
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: "edit capability " + id,
			Validate: func(vctx context.Context, r store.Execer) error {
				cur, ok, e := store.GetCapability(vctx, r, id)
				if e != nil {
					return e
				}
				if !ok {
					return app.NotFound("capability", id)
				}
				if cmd.Flags().Changed("level") {
					if e := app.ValidateEnum("level", capLevel, enums.CapabilityLevel); e != nil {
						return e
					}
					cur.Level = capLevel
				}
				if cmd.Flags().Changed("title") {
					cur.Title = capTitle
				}
				if cmd.Flags().Changed("domain") {
					if _, ok, e := store.DomainIDBySlug(vctx, r, capDomain); e != nil {
						return e
					} else if !ok {
						return app.NotFoundErr(fmt.Errorf("unknown domain %q", capDomain))
					}
					cur.DomainSlug = capDomain
				}
				capRow = cur
				return nil
			},
		}, func(ctx context.Context, w *app.Write) error {
			domainID, _, e := store.DomainIDBySlug(ctx, w.Tx, capRow.DomainSlug)
			if e != nil {
				return e
			}
			if _, e := store.UpsertCapability(ctx, w.Tx, store.Capability{ID: capRow.ID, Title: capRow.Title, Level: capRow.Level, DomainID: domainID}); e != nil {
				return e
			}
			if cmd.Flags().Changed("parent") {
				if e := store.SetCapabilityParent(ctx, w.Tx, id, capParent); e != nil {
					return e
				}
			}
			w.MarkDirty("plan_capability")
			return nil
		})
		if err != nil {
			return err
		}
		emit(capRow, fmt.Sprintf("updated capability %s", id))
		return nil
	},
}

var capabilityDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a capability (its links cascade; sub-capabilities are re-parented to null)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: "delete capability " + args[0],
		}, func(ctx context.Context, w *app.Write) error {
			ok, e := store.DeleteCapability(ctx, w.Tx, args[0])
			if e != nil {
				return e
			}
			if !ok {
				return app.NotFound("capability", args[0])
			}
			for _, t := range []string{"plan_capability", "plan_capability_milestone", "plan_capability_deliverable", "pub_external_ref", "req_edge", "req_entity_ref"} {
				w.MarkDirty(t)
			}
			return nil
		})
		if err != nil {
			return err
		}
		emit(map[string]any{"id": args[0]}, fmt.Sprintf("deleted capability %s", args[0]))
		return nil
	},
}

var capabilityLinkCmd = &cobra.Command{
	Use:   "link <id>",
	Short: "Link a capability to a milestone (--milestone) or deliverable (--deliverable)",
	Args:  cobra.ExactArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return capabilityLink(cmd, args[0], false) },
}

var capabilityUnlinkCmd = &cobra.Command{
	Use:   "unlink <id>",
	Short: "Unlink a capability from a milestone (--milestone) or deliverable (--deliverable)",
	Args:  cobra.ExactArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return capabilityLink(cmd, args[0], true) },
}

func capabilityLink(cmd *cobra.Command, capID string, unlink bool) error {
	ctx := cmd.Context()
	if capMilestone == "" && capDeliverable == "" {
		return app.ValidationFailed(fmt.Errorf("pass --milestone <slug> or --deliverable <id>"))
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
		Summary: verb + " capability " + capID,
		Validate: func(vctx context.Context, r store.Execer) error {
			if _, ok, e := store.GetCapability(vctx, r, capID); e != nil {
				return e
			} else if !ok {
				return app.NotFound("capability", capID)
			}
			return nil
		},
	}, func(ctx context.Context, w *app.Write) error {
		if capMilestone != "" {
			mid, ok, e := store.MilestoneIDBySlug(ctx, w.Tx, capMilestone)
			if e != nil {
				return e
			}
			if !ok {
				return app.NotFound("milestone", capMilestone)
			}
			if unlink {
				e = store.UnlinkCapabilityMilestone(ctx, w.Tx, capID, mid)
			} else {
				e = store.LinkCapabilityMilestone(ctx, w.Tx, capID, mid)
			}
			if e != nil {
				return e
			}
			w.MarkDirty("plan_capability_milestone")
		}
		if capDeliverable != "" {
			if _, ok, e := store.GetDeliverable(ctx, w.Tx, capDeliverable); e != nil {
				return e
			} else if !ok {
				return app.NotFound("deliverable", capDeliverable)
			}
			var e error
			if unlink {
				e = store.UnlinkCapabilityDeliverable(ctx, w.Tx, capID, capDeliverable)
			} else {
				e = store.LinkCapabilityDeliverable(ctx, w.Tx, capID, capDeliverable)
			}
			if e != nil {
				return e
			}
			w.MarkDirty("plan_capability_deliverable")
		}
		return nil
	})
	if err != nil {
		return err
	}
	emit(map[string]any{"capability": capID, "milestone": capMilestone, "deliverable": capDeliverable, "unlinked": unlink},
		fmt.Sprintf("%sed capability %s", verb, capID))
	return nil
}

func init() {
	capabilityAddCmd.Flags().StringVar(&capDomain, "domain", "", "owning domain slug (required)")
	capabilityAddCmd.Flags().StringVar(&capLevel, "level", "", "level (domain|epic|capability)")
	capabilityAddCmd.Flags().StringVar(&capParent, "parent", "", "parent capability id")
	_ = capabilityAddCmd.MarkFlagRequired("domain")
	capabilityEditCmd.Flags().StringVar(&capTitle, "title", "", "new title")
	capabilityEditCmd.Flags().StringVar(&capLevel, "level", "", "level (domain|epic|capability)")
	capabilityEditCmd.Flags().StringVar(&capDomain, "domain", "", "owning domain slug")
	capabilityEditCmd.Flags().StringVar(&capParent, "parent", "", "parent capability id (empty to clear)")
	for _, c := range []*cobra.Command{capabilityLinkCmd, capabilityUnlinkCmd} {
		c.Flags().StringVar(&capMilestone, "milestone", "", "milestone slug")
		c.Flags().StringVar(&capDeliverable, "deliverable", "", "deliverable id")
	}
	capabilityCmd.AddCommand(capabilityAddCmd, capabilityLsCmd, capabilityShowCmd, capabilityEditCmd, capabilityDeleteCmd, capabilityLinkCmd, capabilityUnlinkCmd)
	rootCmd.AddCommand(capabilityCmd)
}
