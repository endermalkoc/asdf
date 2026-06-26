package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/asdf/internal/app"
	"github.com/endermalkoc/asdf/internal/enums"
	"github.com/endermalkoc/asdf/internal/refs"
	"github.com/endermalkoc/asdf/internal/store"
)

var edgeCmd = &cobra.Command{Use: "edge", Short: "Manage the cross-reference graph"}

var edgeAddCmd = &cobra.Command{
	Use:   "add <from> <kind> <to>",
	Short: "Link two entities with a typed edge (deterministic, merge-convergent id)",
	Long: "Add a typed edge between any two keyed entities. Each endpoint is a TYPE:key\n" +
		"reference; a bare value is taken as a requirement fr_key. Examples:\n" +
		"  asdf edge add ATT-FR-002 refines ATT-FR-001\n" +
		"  asdf edge add SPEC:ADDS depends_on ENTITY:Student\n" +
		"  asdf edge add REQ:ATT-FR-012 relates MILESTONE:M4\n" +
		"Both endpoints must already exist (validated on the target branch). For the acyclic\n" +
		"kinds (refines, depends_on, supersedes, defers_to) the edge is rejected if it would\n" +
		"create a cycle. The id is derived from the edge's identity, so re-adding it is a no-op.",
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		from, kind, to := args[0], args[1], args[2]
		var fromT, toT refs.Target
		var id string
		err = app.Mutate(ctx, ws, app.MutateOpts{
			Summary:   fmt.Sprintf("link %s %s %s", from, kind, to),
			Changeset: flagChangeset,
			Actor:     flagActor,
			Validate: func(vctx context.Context, r store.Execer) error {
				if e := app.ValidateEnum("edge kind", kind, enums.EdgeKind); e != nil {
					return e
				}
				resolver, e := app.LoadResolver(vctx, r)
				if e != nil {
					return e
				}
				var ok bool
				if fromT, ok = app.ResolveRef(resolver, from); !ok {
					return fmt.Errorf("unknown from-endpoint %q — use TYPE:key (e.g. SPEC:ADDS, ENTITY:Student, REQ:ATT-FR-012)", from)
				}
				if toT, ok = app.ResolveRef(resolver, to); !ok {
					return fmt.Errorf("unknown to-endpoint %q — use TYPE:key (e.g. SPEC:ADDS, ENTITY:Student, REQ:ATT-FR-012)", to)
				}
				return app.CheckEdgeAcyclic(vctx, r, fromT.Type, fromT.ID, kind, toT.Type, toT.ID, from, to)
			},
		}, func(ctx context.Context, w *app.Write) error {
			res, e := store.AddEdgeByIDs(ctx, w.Tx, fromT.Type, fromT.ID, kind, toT.Type, toT.ID)
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
		fromLabel := fmt.Sprintf("%s:%s", fromT.Type, fromT.Key)
		toLabel := fmt.Sprintf("%s:%s", toT.Type, toT.Key)
		emit(map[string]string{"id": id, "from": fromLabel, "kind": kind, "to": toLabel},
			fmt.Sprintf("edge: %s -%s-> %s  (id=%s)", fromLabel, kind, toLabel, id))
		return nil
	},
}

func init() {
	edgeCmd.AddCommand(edgeAddCmd)
	rootCmd.AddCommand(edgeCmd)
}
