package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/generate"
)

var (
	generateOut    string
	generateFormat string
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate read artifacts from the database (Markdown/JSON/HTML; git-ignored)",
	Long: "Reconstruct read artifacts from the canonical Dolt database, one file per spec/entity\n" +
		"at its source-relative path plus index pages. --format selects the renderer: markdown\n" +
		"(Obsidian, default), json (structured records + an index.json manifest), or html.\n" +
		"Output is a build artifact — never hand-edit it; change the DB and regenerate. Reads only.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		st, err := generate.Generate(ctx, ws.DB(), generateOut, generateFormat)
		if err != nil {
			return err
		}
		if flagJSON {
			emit(st, "")
			return nil
		}
		fmt.Printf("generated %d %s files into %s  (specs %d, entity docs %d, indexes %d, glossary %d)\n",
			st.Total(), st.Format, st.OutDir, st.Specs, st.Entities, st.Indexes, st.Glossary)
		return nil
	},
}

func init() {
	generateCmd.Flags().StringVar(&generateOut, "out", ".cusp/generated",
		"output directory for generated artifacts")
	generateCmd.Flags().StringVar(&generateFormat, "format", "md",
		"output format: md | json | html")
	rootCmd.AddCommand(generateCmd)
}
