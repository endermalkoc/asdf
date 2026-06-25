package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/asdf/internal/generate"
)

var generateOut string

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate Markdown from the database (git-ignored, read-only build artifacts)",
	Long: "Reconstruct Markdown from the canonical Dolt database: one file per spec at its\n" +
		"source-relative path, plus the domain and entity index pages. Output is a build\n" +
		"artifact — never hand-edit it; change the DB and regenerate. Reads only.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		st, err := generate.Generate(ctx, ws.DB(), generateOut)
		if err != nil {
			return err
		}
		if flagJSON {
			emit(st, "")
			return nil
		}
		fmt.Printf("generated %d files into %s  (specs %d, entity docs %d, indexes %d, glossary %d)\n",
			st.Total(), st.OutDir, st.Specs, st.Entities, st.Indexes, st.Glossary)
		return nil
	},
}

func init() {
	generateCmd.Flags().StringVar(&generateOut, "out", ".asdf/generated",
		"output directory for generated Markdown")
	rootCmd.AddCommand(generateCmd)
}
