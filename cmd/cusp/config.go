package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/workspace"
)

var configOutDir string

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Read and edit the workspace config (.cusp/config.json)",
	Long: "Manage project-local workspace settings. The `generate` section controls incremental\n" +
		"auto-generation: when enabled, every change committed to main re-materializes only the\n" +
		"affected documents in each configured format. Output defaults to .cusp/artifacts/<format>.",
	RunE: func(cmd *cobra.Command, args []string) error { return showConfig() },
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the workspace config",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, args []string) error { return showConfig() },
}

var configGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Configure incremental auto-generation (enable/disable, formats, sync)",
	RunE:  func(cmd *cobra.Command, args []string) error { return showConfig() },
}

var configGenerateEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Turn on auto-generation on every change committed to main",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return editConfig(func(c *workspace.Config) error {
			c.Generate.Enabled = true
			if len(c.Generate.Formats) == 0 {
				fmt.Fprintln(os.Stderr, "note: no formats configured yet — add one with `cusp config generate add <format>`")
			}
			return nil
		}, "auto-generation enabled (run `cusp config generate sync` to materialize now)")
	},
}

var configGenerateDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Turn off auto-generation (formats stay configured)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return editConfig(func(c *workspace.Config) error {
			c.Generate.Enabled = false
			return nil
		}, "auto-generation disabled")
	},
}

var configGenerateAddCmd = &cobra.Command{
	Use:   "add <format>",
	Short: "Add (or re-point) an auto-generated format; --out overrides .cusp/artifacts/<format>",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		format, err := canonicalFormat(args[0])
		if err != nil {
			return err
		}
		return editConfig(func(c *workspace.Config) error {
			fc := workspace.FormatConfig{Format: format, Out: strings.TrimSpace(configOutDir)}
			replaced := false
			for i := range c.Generate.Formats {
				if c.Generate.Formats[i].Format == format {
					c.Generate.Formats[i] = fc
					replaced = true
					break
				}
			}
			if !replaced {
				c.Generate.Formats = append(c.Generate.Formats, fc)
			}
			return nil
		}, fmt.Sprintf("format %q configured (run `cusp config generate sync` to materialize now)", format))
	},
}

var configGenerateRemoveCmd = &cobra.Command{
	Use:   "remove <format>",
	Short: "Stop auto-generating a format (does not delete already-written files)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		format, err := canonicalFormat(args[0])
		if err != nil {
			return err
		}
		return editConfig(func(c *workspace.Config) error {
			kept := c.Generate.Formats[:0]
			for _, f := range c.Generate.Formats {
				if f.Format != format {
					kept = append(kept, f)
				}
			}
			c.Generate.Formats = kept
			return nil
		}, fmt.Sprintf("format %q removed", format))
	},
}

var configGenerateSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Rebuild every configured format from main in full (orphans removed)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		st, err := app.SyncConfiguredFull(ctx, ws)
		if err != nil {
			return err
		}
		if flagJSON {
			emit(st, "")
			return nil
		}
		if len(st.Formats) == 0 {
			fmt.Println("no formats configured — add one with `cusp config generate add <format>`")
			return nil
		}
		fmt.Printf("synced %s: %d written, %d removed\n", strings.Join(st.Formats, ", "), st.Written, st.Removed)
		return nil
	},
}

func init() {
	configGenerateAddCmd.Flags().StringVar(&configOutDir, "out", "", "output directory (default .cusp/artifacts/<format>)")
	configGenerateCmd.AddCommand(configGenerateEnableCmd, configGenerateDisableCmd, configGenerateAddCmd, configGenerateRemoveCmd, configGenerateSyncCmd)
	configCmd.AddCommand(configShowCmd, configGenerateCmd)
	rootCmd.AddCommand(configCmd)
}

// canonicalFormat validates and normalizes a format token to md | json | html.
func canonicalFormat(format string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "md", "markdown":
		return "md", nil
	case "json":
		return "json", nil
	case "html":
		return "html", nil
	default:
		return "", fmt.Errorf("unknown format %q (want: md, json, html)", format)
	}
}

// configDir resolves the workspace `.cusp` directory for config-file edits, which need no
// database connection. It requires the workspace to exist.
func configDir() (string, error) {
	cuspDir, err := workspace.ResolveCuspDir()
	if err != nil {
		return "", err
	}
	if _, statErr := os.Stat(cuspDir); statErr != nil {
		return "", fmt.Errorf("no Cusp workspace at %s — run `cusp init` first", cuspDir)
	}
	return cuspDir, nil
}

func showConfig() error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	cfg, err := workspace.LoadConfigDir(dir)
	if err != nil {
		return err
	}
	if flagJSON {
		emit(cfg, "")
		return nil
	}
	g := cfg.Generate
	state := "disabled"
	if g.Enabled {
		state = "enabled"
	}
	fmt.Printf("generate: %s\n", state)
	if len(g.Formats) == 0 {
		fmt.Println("  formats: (none)")
		return nil
	}
	fmt.Println("  formats:")
	for _, f := range g.Formats {
		marker := ""
		if strings.TrimSpace(f.Out) != "" {
			marker = "  (override)"
		}
		fmt.Printf("    - %-5s → %s%s\n", f.Format, workspace.EffectiveOutDir(dir, f), marker)
	}
	return nil
}

// editConfig loads, mutates, and saves the workspace config, then prints msg (unless --json,
// which re-emits the saved config).
func editConfig(mutate func(*workspace.Config) error, msg string) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	cfg, err := workspace.LoadConfigDir(dir)
	if err != nil {
		return err
	}
	if err := mutate(cfg); err != nil {
		return err
	}
	if err := workspace.SaveConfigDir(dir, cfg); err != nil {
		return err
	}
	if flagJSON {
		emit(cfg, "")
		return nil
	}
	fmt.Println(msg)
	return nil
}
