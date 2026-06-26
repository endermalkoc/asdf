package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/asdf/internal/store"
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

func init() {
	entityCmd.AddCommand(entityLsCmd)
	rootCmd.AddCommand(entityCmd)
}
