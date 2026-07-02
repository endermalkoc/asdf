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
	msDescription string
	msStatus      string
	msSequence    int
)

var milestoneCmd = &cobra.Command{Use: "milestone", Aliases: []string{"ms"}, Short: "Manage milestones (planning)"}

func msSeqArg(cmd *cobra.Command) *int {
	if cmd.Flags().Changed("sequence") {
		v := msSequence
		return &v
	}
	return nil
}

var milestoneAddCmd = &cobra.Command{
	Use:   "add <slug> <name>",
	Short: "Add a milestone (slug is its stable key, e.g. M8)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		slug, name := args[0], args[1]
		var m store.MilestoneRow
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: "add milestone " + slug,
			Validate: func(vctx context.Context, r store.Execer) error {
				return app.ValidateEnum("status", msStatus, enums.MilestoneStatus)
			},
		}, func(ctx context.Context, w *app.Write) error {
			var e error
			m, e = store.AddMilestone(ctx, w.Tx, store.MilestoneRow{
				Slug: slug, Name: name, Description: msDescription, Sequence: msSeqArg(cmd), Status: msStatus,
			})
			if e != nil {
				return e
			}
			w.MarkDirty("plan_milestone")
			return nil
		})
		if err != nil {
			return err
		}
		emit(m, fmt.Sprintf("added milestone %s — %s  (id=%s)", m.Slug, m.Name, m.ID))
		return nil
	},
}

var milestoneLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List milestones",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		r, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		ms, err := store.ListMilestones(ctx, r)
		if err != nil {
			return err
		}
		if flagJSON {
			emit(ms, "")
			return nil
		}
		var b strings.Builder
		for _, m := range ms {
			fmt.Fprintf(&b, "%-12s %-13s %s\n", m.Slug, m.Status, m.Name)
		}
		fmt.Print(b.String())
		return nil
	},
}

var milestoneShowCmd = &cobra.Command{
	Use:   "show <slug>",
	Short: "Show a milestone's fields",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		r, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		m, ok, err := store.GetMilestone(ctx, r, args[0])
		if err != nil {
			return err
		}
		if !ok {
			return app.NotFound("milestone", args[0])
		}
		if flagJSON {
			emit(m, "")
			return nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s  (%s)\n", m.Slug, m.Status)
		fmt.Fprintf(&b, "  name:        %s\n", m.Name)
		if m.Description != "" {
			fmt.Fprintf(&b, "  description: %s\n", m.Description)
		}
		if m.Sequence != nil {
			fmt.Fprintf(&b, "  sequence:    %d\n", *m.Sequence)
		}
		fmt.Fprintf(&b, "  id:          %s\n", m.ID)
		fmt.Print(b.String())
		return nil
	},
}

var milestoneEditCmd = &cobra.Command{
	Use:   "edit <slug>",
	Short: "Edit a milestone's name/description/sequence/status (only the flags you pass change)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		if cmd.Flags().NFlag() == 0 {
			return fmt.Errorf("nothing to edit — pass --name/--description/--sequence/--status")
		}
		var m store.MilestoneRow
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: "edit milestone " + args[0],
			Validate: func(vctx context.Context, r store.Execer) error {
				cur, ok, e := store.GetMilestone(vctx, r, args[0])
				if e != nil {
					return e
				}
				if !ok {
					return app.NotFound("milestone", args[0])
				}
				if cmd.Flags().Changed("status") {
					if e := app.ValidateEnum("status", msStatus, enums.MilestoneStatus); e != nil {
						return e
					}
					cur.Status = msStatus
				}
				if cmd.Flags().Changed("name") {
					cur.Name = milestoneName
				}
				if cmd.Flags().Changed("description") {
					cur.Description = msDescription
				}
				if cmd.Flags().Changed("sequence") {
					cur.Sequence = msSeqArg(cmd)
				}
				m = cur
				return nil
			},
		}, func(ctx context.Context, w *app.Write) error {
			if e := store.UpdateMilestone(ctx, w.Tx, m); e != nil {
				return e
			}
			w.MarkDirty("plan_milestone")
			return nil
		})
		if err != nil {
			return err
		}
		emit(m, fmt.Sprintf("updated milestone %s", m.Slug))
		return nil
	},
}

var milestoneDeleteCmd = &cobra.Command{
	Use:   "delete <slug>",
	Short: "Delete a milestone (deliverables/runs referencing it FK-null out)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		var deleted store.MilestoneRow
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: "delete milestone " + args[0],
		}, func(ctx context.Context, w *app.Write) error {
			m, ok, e := store.DeleteMilestone(ctx, w.Tx, args[0])
			if e != nil {
				return e
			}
			if !ok {
				return app.NotFound("milestone", args[0])
			}
			deleted = m
			w.MarkDirty("plan_milestone")
			return nil
		})
		if err != nil {
			return err
		}
		emit(deleted, fmt.Sprintf("deleted milestone %s", deleted.Slug))
		return nil
	},
}

var milestoneName string

func init() {
	milestoneAddCmd.Flags().StringVar(&msDescription, "description", "", "description")
	milestoneAddCmd.Flags().StringVar(&msStatus, "status", "", "status (complete|in_progress|pending; default pending)")
	milestoneAddCmd.Flags().IntVar(&msSequence, "sequence", 0, "ordering sequence")
	milestoneEditCmd.Flags().StringVar(&milestoneName, "name", "", "new name")
	milestoneEditCmd.Flags().StringVar(&msDescription, "description", "", "new description")
	milestoneEditCmd.Flags().StringVar(&msStatus, "status", "", "status (complete|in_progress|pending)")
	milestoneEditCmd.Flags().IntVar(&msSequence, "sequence", 0, "ordering sequence")
	milestoneCmd.AddCommand(milestoneAddCmd, milestoneLsCmd, milestoneShowCmd, milestoneEditCmd, milestoneDeleteCmd)
	rootCmd.AddCommand(milestoneCmd)
}
