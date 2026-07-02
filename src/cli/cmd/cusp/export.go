package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
)

var exportOut string

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export the whole graph as JSON Lines (a git-friendly snapshot)",
	Long: "Dumps every data table as JSON Lines — one {\"table\":…,\"row\":{…}} object per line,\n" +
		"tables in name order and rows totally ordered, so the output is deterministic and\n" +
		"diffable. Reads the active changeset (else main). Writes to stdout, or --out <file>.\n" +
		"Every value is a string or null (types recover from the schema); useful for backups\n" +
		"and interop.\n\n" +
		"  cusp export                 # JSONL to stdout\n" +
		"  cusp export --out snap.jsonl",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		r, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()

		toFile := exportOut != "" && exportOut != "-"
		var w *bufio.Writer
		closeFn := func() error { return nil }
		if toFile {
			f, err := os.Create(exportOut)
			if err != nil {
				return err
			}
			w = bufio.NewWriter(f)
			closeFn = f.Close
		} else {
			w = bufio.NewWriter(os.Stdout)
		}

		stats, err := app.Export(ctx, r, w)
		if err != nil {
			return err
		}
		if err := w.Flush(); err != nil {
			_ = closeFn()
			return err
		}
		if err := closeFn(); err != nil {
			return err
		}

		if toFile {
			emit(map[string]any{"out": exportOut, "tables": stats.Tables, "rows": stats.Rows},
				fmt.Sprintf("exported %d rows across %d tables → %s", stats.Rows, stats.Tables, exportOut))
		} else {
			// stdout carries the JSONL; keep the summary off it so pipes stay clean.
			fmt.Fprintf(os.Stderr, "exported %d rows across %d tables\n", stats.Rows, stats.Tables)
		}
		return nil
	},
}

func init() {
	exportCmd.Flags().StringVar(&exportOut, "out", "", "write to this file instead of stdout")
	rootCmd.AddCommand(exportCmd)
}
