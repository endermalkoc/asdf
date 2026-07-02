package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/configfile"
	"github.com/endermalkoc/cusp/internal/doltserver"
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

// cfgEntry is one effective-config key/value for `config get`.
type cfgEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Print effective config — actor identity, Dolt server, generate (all, or one key)",
	Long: "Print the effective configuration: the resolved actor identity (what writes are\n" +
		"attributed to), the Dolt server settings (mode/database/host/port/user), and the\n" +
		"generate config. With a key (e.g. `dolt.mode`, `user.handle`), prints just that value.\n" +
		"Read-only; settable keys are the `user.*` identity ones via `cusp config set`.",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := configDir()
		if err != nil {
			return err
		}
		entries, err := effectiveConfig(dir)
		if err != nil {
			return err
		}
		if len(args) == 1 {
			for _, e := range entries {
				if e.Key == args[0] {
					if flagJSON {
						emit(e, "")
					} else {
						fmt.Println(e.Value)
					}
					return nil
				}
			}
			return app.NotFound("config key", args[0])
		}
		if flagJSON {
			emit(entries, "")
			return nil
		}
		for _, e := range entries {
			fmt.Printf("%-20s %s\n", e.Key, e.Value)
		}
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value (settable: user.handle, user.name, user.email)",
	Long: "Persist a config value. Only actor-identity keys are settable here — they save to a\n" +
		"per-user, git-ignored .cusp/identity.json, so your writes are attributed without passing\n" +
		"--actor every time (and your identity isn't shared through the host repo):\n" +
		"  user.handle   stable identity, e.g. emalkoc\n" +
		"  user.name     display name\n" +
		"  user.email    Dolt commit email\n\n" +
		"Generate settings have their own verbs (`cusp config generate …`); Dolt server settings\n" +
		"are managed by the workspace and are read-only via `cusp config get`.",
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := configDir()
		if err != nil {
			return err
		}
		key, val := args[0], args[1]
		id, err := workspace.LoadIdentity(dir)
		if err != nil {
			return err
		}
		switch key {
		case "user.handle":
			id.Handle = val
		case "user.name":
			id.Name = val
		case "user.email":
			id.Email = val
		default:
			return app.ValidationFailed(fmt.Errorf(
				"key %q is not settable — settable: user.handle, user.name, user.email "+
					"(generate: `cusp config generate …`; Dolt server settings are workspace-managed)", key))
		}
		if err := workspace.SaveIdentity(dir, id); err != nil {
			return err
		}
		if flagJSON {
			emit(map[string]string{"key": key, "value": val}, "")
			return nil
		}
		fmt.Printf("set %s = %s\n", key, val)
		return nil
	},
}

// effectiveConfig builds the resolved config view for `config get`, all without a database
// connection: actor identity (ResolveActor), Dolt server metadata (metadata.json + live
// server state), and the workspace generate config (config.json).
func effectiveConfig(cuspDir string) ([]cfgEntry, error) {
	var out []cfgEntry
	add := func(k, v string) { out = append(out, cfgEntry{Key: k, Value: v}) }

	actor := workspace.ResolveActor(flagActor)
	add("user.handle", actor.Handle)
	add("user.name", actor.Name)
	add("user.email", actor.Email)

	meta, err := configfile.Load(cuspDir)
	if err != nil {
		return nil, err
	}
	if flagDSN != "" {
		add("dolt.mode", "external")
		add("dolt.dsn", flagDSN)
	} else {
		add("dolt.mode", doltserver.ResolveServerMode(cuspDir).String())
		if state, e := doltserver.IsRunning(doltserver.ResolveServerDir(cuspDir)); e == nil && state.Running {
			add("dolt.server.port", fmt.Sprintf("%d", state.Port))
		} else {
			add("dolt.server.port", "(server not running)")
		}
	}
	add("dolt.database", meta.GetDoltDatabase())
	add("dolt.server.host", meta.GetDoltServerHost())
	add("dolt.server.user", meta.GetDoltServerUser())

	wc, err := workspace.LoadConfigDir(cuspDir)
	if err != nil {
		return nil, err
	}
	add("generate.enabled", fmt.Sprintf("%t", wc.Generate.Enabled))
	var formats []string
	for _, f := range wc.Generate.Formats {
		formats = append(formats, f.Format)
	}
	add("generate.formats", strings.Join(formats, ","))
	return out, nil
}

func init() {
	configGenerateAddCmd.Flags().StringVar(&configOutDir, "out", "", "output directory (default .cusp/artifacts/<format>)")
	configGenerateCmd.AddCommand(configGenerateEnableCmd, configGenerateDisableCmd, configGenerateAddCmd, configGenerateRemoveCmd, configGenerateSyncCmd)
	configCmd.AddCommand(configShowCmd, configGenerateCmd, configGetCmd, configSetCmd)
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
