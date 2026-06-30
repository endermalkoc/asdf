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
	entityDescription string
	entityStatus      string
)

// entityCmd groups entity reads + the entity section / section-type verbs (the section
// trees are attached in section.go). Entities themselves are created by import today.
var entityCmd = &cobra.Command{Use: "entity", Short: "Inspect entities and manage their doc sections"}

var entityLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List entities",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		r, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		ents, err := store.ListEntities(ctx, r)
		if err != nil {
			return err
		}
		if flagJSON {
			emit(ents, "")
			return nil
		}
		var b strings.Builder
		for _, e := range ents {
			fmt.Fprintf(&b, "%-28s %s\n", e.Name, e.Status)
		}
		fmt.Print(b.String())
		return nil
	},
}

var entityShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show an entity's fields",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		rd, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		e, ok, err := store.GetEntity(ctx, rd, args[0])
		if err != nil {
			return err
		}
		if !ok {
			return app.NotFound("entity", args[0])
		}
		if flagJSON {
			emit(e, "")
			return nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s  (%s)\n", e.Name, e.Status)
		if e.Description != "" {
			fmt.Fprintf(&b, "  description: %s\n", e.Description)
		}
		fmt.Fprintf(&b, "  doc:         %s\n", e.DocPath)
		fmt.Fprintf(&b, "  id:          %s\n", e.ID)
		fmt.Print(b.String())
		return nil
	},
}

var entityEditCmd = &cobra.Command{
	Use:   "edit <name>",
	Short: "Edit an entity's description/status (only the flags you pass change)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		if cmd.Flags().NFlag() == 0 {
			return fmt.Errorf("nothing to edit — pass --description and/or --status")
		}
		var e store.EntityRow
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: "edit entity " + args[0],
			Validate: func(vctx context.Context, r store.Execer) error {
				cur, ok, er := store.GetEntity(vctx, r, args[0])
				if er != nil {
					return er
				}
				if !ok {
					return app.NotFound("entity", args[0])
				}
				if cmd.Flags().Changed("status") {
					if er := app.ValidateEnum("status", entityStatus, enums.EntityStatus); er != nil {
						return er
					}
					cur.Status = entityStatus
				}
				if cmd.Flags().Changed("description") {
					cur.Description = entityDescription
				}
				e = cur
				return nil
			},
		}, func(ctx context.Context, w *app.Write) error {
			if er := store.UpdateEntity(ctx, w.Tx, e.ID, e.Description, e.Status); er != nil {
				return er
			}
			w.MarkDirty("ent_entity")
			return nil
		})
		if err != nil {
			return err
		}
		emit(e, fmt.Sprintf("updated entity %s", e.Name))
		return nil
	},
}

var entityDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete an entity and its sections, relationships, and references",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		var deleted store.EntityRow
		err = runMutate(cmd, ws, app.MutateOpts{
			Summary: "delete entity " + args[0],
		}, func(ctx context.Context, w *app.Write) error {
			e, ok, er := store.DeleteEntity(ctx, w.Tx, args[0])
			if er != nil {
				return er
			}
			if !ok {
				return app.NotFound("entity", args[0])
			}
			deleted = e
			for _, t := range []string{"ent_entity", "ent_entity_section", "ent_relationship", "req_entity_ref", "req_edge", "pub_external_ref"} {
				w.MarkDirty(t)
			}
			return nil
		})
		if err != nil {
			return err
		}
		emit(deleted, fmt.Sprintf("deleted entity %s", deleted.Name))
		return nil
	},
}

func init() {
	entityEditCmd.Flags().StringVar(&entityDescription, "description", "", "new description")
	entityEditCmd.Flags().StringVar(&entityStatus, "status", "", "status (draft|active|deprecated)")
	entityCmd.AddCommand(entityLsCmd, entityShowCmd, entityEditCmd, entityDeleteCmd)
	rootCmd.AddCommand(entityCmd)
}
