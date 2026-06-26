package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/asdf/internal/app"
	"github.com/endermalkoc/asdf/internal/enums"
	"github.com/endermalkoc/asdf/internal/store"
)

var edgeCmd = &cobra.Command{Use: "edge", Short: "Manage the cross-reference graph"}

var edgeAddCmd = &cobra.Command{
	Use:   "add <from-fr> <kind> <to-fr>",
	Short: "Link two requirements (deterministic, merge-convergent id)",
	Long: "Add a typed edge between two requirements by their FR keys, e.g.\n" +
		"  asdf edge add ATT-FR-002 refines ATT-FR-001\n" +
		"The edge's id is derived from its identity, so re-adding the same edge is a no-op.",
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		from, kind, to := args[0], args[1], args[2]
		var id string
		err = app.Mutate(ctx, ws, app.MutateOpts{
			Summary:   fmt.Sprintf("link %s %s %s", from, kind, to),
			Changeset: flagChangeset,
			Actor:     flagActor,
			Validate:  func(context.Context) error { return app.ValidateEnum("edge kind", kind, enums.EdgeKind) },
		}, func(ctx context.Context, w *app.Write) error {
			res, e := store.AddEdge(ctx, w.Tx, from, kind, to)
			if e != nil {
				return e
			}
			w.MarkDirty("req_edge")
			id = res
			return nil
		})
		if err != nil {
			return err
		}
		emit(map[string]string{"id": id, "from": from, "kind": kind, "to": to},
			fmt.Sprintf("edge: %s -%s-> %s  (id=%s)", from, kind, to, id))
		return nil
	},
}

func init() {
	edgeCmd.AddCommand(edgeAddCmd)
	rootCmd.AddCommand(edgeCmd)
}
