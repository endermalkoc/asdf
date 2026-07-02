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

// ---- test config ----------------------------------------------------------

var (
	tcfgGroup string
	tcfgName  string
	tcfgDesc  string
)

var testConfigCmd = &cobra.Command{Use: "config", Short: "Manage test configurations (group/value pairs)"}

var testConfigAddCmd = &cobra.Command{
	Use: "add <group> <name>", Short: "Add a configuration (e.g. browser chrome)", Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleMutate(cmd, "add configuration "+args[0]+"/"+args[1], nil,
			func(ctx context.Context, w *app.Write) (any, string, error) {
				c, e := store.AddConfiguration(ctx, w.Tx, store.ConfigurationRow{Group: args[0], Name: args[1], Description: tcfgDesc})
				if e != nil {
					return nil, "", e
				}
				w.MarkDirty("test_configuration")
				return c, fmt.Sprintf("added configuration %s/%s  (id=%s)", c.Group, c.Name, c.ID), nil
			})
	},
}

var testConfigLsCmd = &cobra.Command{
	Use: "ls", Short: "List configurations", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleList(cmd, func(ctx context.Context, r store.Execer) (any, string, error) {
			cfgs, e := store.ListConfigurations(ctx, r)
			if e != nil {
				return nil, "", e
			}
			var b strings.Builder
			for _, c := range cfgs {
				fmt.Fprintf(&b, "%-26s %-16s %s\n", c.ID, c.Group, c.Name)
			}
			return cfgs, b.String(), nil
		})
	},
}

var testConfigShowCmd = &cobra.Command{
	Use: "show <id>", Short: "Show a configuration", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleGet(cmd, store.GetConfiguration, "configuration", args[0])
	},
}

var testConfigEditCmd = &cobra.Command{
	Use: "edit <id>", Short: "Edit a configuration (only the flags you pass change)", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().NFlag() == 0 {
			return fmt.Errorf("nothing to edit — pass --group/--name/--description")
		}
		var c store.ConfigurationRow
		return simpleMutate(cmd, "edit configuration "+args[0], func(vctx context.Context, r store.Execer) error {
			cur, ok, e := store.GetConfiguration(vctx, r, args[0])
			if e != nil {
				return e
			}
			if !ok {
				return app.NotFound("configuration", args[0])
			}
			applyStr(cmd, "group", tcfgGroup, &cur.Group)
			applyStr(cmd, "name", tcfgName, &cur.Name)
			applyStr(cmd, "description", tcfgDesc, &cur.Description)
			c = cur
			return nil
		}, func(ctx context.Context, w *app.Write) (any, string, error) {
			if e := store.UpdateConfiguration(ctx, w.Tx, c); e != nil {
				return nil, "", e
			}
			w.MarkDirty("test_configuration")
			return c, "updated configuration " + c.ID, nil
		})
	},
}

var testConfigDeleteCmd = &cobra.Command{
	Use: "delete <id>", Short: "Delete a configuration", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleDelete(cmd, "delete configuration "+args[0], "configuration", args[0],
			store.DeleteConfiguration, "test_configuration", "test_run_configuration")
	},
}

// ---- test run -------------------------------------------------------------

var (
	trunDesc      string
	trunStatus    string
	trunMilestone string
	trunTitle     string // edit
	trunConfig    string // config/unconfig
)

var testRunCmd = &cobra.Command{Use: "run", Short: "Manage test runs (execution cycles)"}

var testRunAddCmd = &cobra.Command{
	Use: "add <title>", Short: "Open a test run", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleMutate(cmd, "add test run "+args[0], func(vctx context.Context, r store.Execer) error {
			if e := app.ValidateEnum("status", trunStatus, enums.TestRunStatus); e != nil {
				return e
			}
			_, e := resolveMilestone(vctx, r, trunMilestone)
			return e
		}, func(ctx context.Context, w *app.Write) (any, string, error) {
			mid, e := resolveMilestone(ctx, w.Tx, trunMilestone)
			if e != nil {
				return nil, "", e
			}
			run, e := store.AddTestRun(ctx, w.Tx, store.TestRunRow{Title: args[0], Description: trunDesc, Status: trunStatus}, mid)
			if e != nil {
				return nil, "", e
			}
			w.MarkDirty("test_run")
			return run, fmt.Sprintf("opened test run %q  (id=%s)", run.Title, run.ID), nil
		})
	},
}

var testRunLsCmd = &cobra.Command{
	Use: "ls", Short: "List test runs", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleList(cmd, func(ctx context.Context, r store.Execer) (any, string, error) {
			runs, e := store.ListTestRuns(ctx, r)
			if e != nil {
				return nil, "", e
			}
			var b strings.Builder
			for _, run := range runs {
				fmt.Fprintf(&b, "%-26s %-10s %-8s %s\n", run.ID, run.Status, run.MilestoneSlug, run.Title)
			}
			return runs, b.String(), nil
		})
	},
}

var testRunShowCmd = &cobra.Command{
	Use: "show <id>", Short: "Show a test run", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleGet(cmd, store.GetTestRun, "test run", args[0])
	},
}

var testRunEditCmd = &cobra.Command{
	Use: "edit <id>", Short: "Edit a test run (only the flags you pass change)", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().NFlag() == 0 {
			return fmt.Errorf("nothing to edit — pass --title/--description/--status/--milestone")
		}
		var run store.TestRunRow
		return simpleMutate(cmd, "edit test run "+args[0], func(vctx context.Context, r store.Execer) error {
			cur, ok, e := store.GetTestRun(vctx, r, args[0])
			if e != nil {
				return e
			}
			if !ok {
				return app.NotFound("test run", args[0])
			}
			if cmd.Flags().Changed("status") {
				if e := app.ValidateEnum("status", trunStatus, enums.TestRunStatus); e != nil {
					return e
				}
				cur.Status = trunStatus
			}
			applyStr(cmd, "title", trunTitle, &cur.Title)
			applyStr(cmd, "description", trunDesc, &cur.Description)
			if cmd.Flags().Changed("milestone") {
				if _, e := resolveMilestone(vctx, r, trunMilestone); e != nil {
					return e
				}
				cur.MilestoneSlug = trunMilestone
			}
			run = cur
			return nil
		}, func(ctx context.Context, w *app.Write) (any, string, error) {
			mid, e := resolveMilestone(ctx, w.Tx, run.MilestoneSlug)
			if e != nil {
				return nil, "", e
			}
			if e := store.UpdateTestRun(ctx, w.Tx, run, mid); e != nil {
				return nil, "", e
			}
			w.MarkDirty("test_run")
			return run, "updated test run " + run.ID, nil
		})
	},
}

var testRunDeleteCmd = &cobra.Command{
	Use: "delete <id>", Short: "Delete a test run (its results cascade)", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleDelete(cmd, "delete test run "+args[0], "test run", args[0],
			store.DeleteTestRun, "test_run", "test_result", "test_run_configuration")
	},
}

var testRunConfigCmd = &cobra.Command{
	Use: "config <run-id>", Short: "Attach a configuration to a run (--config <id>)", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error { return testRunConfig(cmd, args[0], false) },
}
var testRunUnconfigCmd = &cobra.Command{
	Use: "unconfig <run-id>", Short: "Detach a configuration from a run (--config <id>)", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error { return testRunConfig(cmd, args[0], true) },
}

func testRunConfig(cmd *cobra.Command, runID string, remove bool) error {
	if trunConfig == "" {
		return app.ValidationFailed(fmt.Errorf("pass --config <configuration-id>"))
	}
	verb := "attach"
	if remove {
		verb = "detach"
	}
	return simpleMutate(cmd, verb+" config on run "+runID, func(vctx context.Context, r store.Execer) error {
		if _, ok, e := store.GetTestRun(vctx, r, runID); e != nil {
			return e
		} else if !ok {
			return app.NotFound("test run", runID)
		}
		if _, ok, e := store.GetConfiguration(vctx, r, trunConfig); e != nil {
			return e
		} else if !ok {
			return app.NotFound("configuration", trunConfig)
		}
		return nil
	}, func(ctx context.Context, w *app.Write) (any, string, error) {
		var e error
		if remove {
			e = store.UnlinkRunConfiguration(ctx, w.Tx, runID, trunConfig)
		} else {
			e = store.LinkRunConfiguration(ctx, w.Tx, runID, trunConfig)
		}
		if e != nil {
			return nil, "", e
		}
		w.MarkDirty("test_run_configuration")
		return map[string]any{"run": runID, "configuration": trunConfig, "removed": remove},
			fmt.Sprintf("%sed configuration on run %s", verb, runID), nil
	})
}

// ---- test result ----------------------------------------------------------

var (
	tresRun      string
	tresCase     string
	tresConfig   string
	tresStatus   string
	tresComment  string
	tresDuration int
	tresBy       string
)

var testResultCmd = &cobra.Command{Use: "result", Short: "Record and list test results"}

var testResultAddCmd = &cobra.Command{
	Use: "add", Short: "Record a test result (idempotent per run+case+config)", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleMutate(cmd, "record test result", func(vctx context.Context, r store.Execer) error {
			if e := app.ValidateEnum("status", tresStatus, enums.TestResultStatus); e != nil {
				return e
			}
			if _, ok, e := store.GetTestRun(vctx, r, tresRun); e != nil {
				return e
			} else if !ok {
				return app.NotFound("test run", tresRun)
			}
			if _, ok, e := store.GetTestCase(vctx, r, tresCase); e != nil {
				return e
			} else if !ok {
				return app.NotFound("test case", tresCase)
			}
			if tresConfig != "" {
				if _, ok, e := store.GetConfiguration(vctx, r, tresConfig); e != nil {
					return e
				} else if !ok {
					return app.NotFound("configuration", tresConfig)
				}
			}
			return nil
		}, func(ctx context.Context, w *app.Write) (any, string, error) {
			by := tresBy
			if by == "" {
				by = w.Actor.Handle
			}
			res, e := store.UpsertTestResult(ctx, w.Tx, store.TestResultRow{
				RunID: tresRun, TestCaseID: tresCase, ConfigurationID: tresConfig,
				Status: tresStatus, Comment: tresComment, DurationMs: intFlagArg(cmd, "duration", tresDuration), ExecutedBy: by,
			})
			if e != nil {
				return nil, "", e
			}
			w.MarkDirty("test_result")
			return res, fmt.Sprintf("recorded %s for case %s in run %s", tresStatus, tresCase, tresRun), nil
		})
	},
}

var testResultLsCmd = &cobra.Command{
	Use: "ls <run-id>", Short: "List a run's results", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleList(cmd, func(ctx context.Context, r store.Execer) (any, string, error) {
			results, e := store.ListTestResults(ctx, r, args[0])
			if e != nil {
				return nil, "", e
			}
			var b strings.Builder
			for _, res := range results {
				fmt.Fprintf(&b, "%-10s case=%s  %s\n", res.Status, res.TestCaseID, oneLineComment(res.Comment))
			}
			return results, b.String(), nil
		})
	},
}

func init() {
	testConfigAddCmd.Flags().StringVar(&tcfgDesc, "description", "", "description")
	testConfigEditCmd.Flags().StringVar(&tcfgGroup, "group", "", "group")
	testConfigEditCmd.Flags().StringVar(&tcfgName, "name", "", "value")
	testConfigEditCmd.Flags().StringVar(&tcfgDesc, "description", "", "description")
	testConfigCmd.AddCommand(testConfigAddCmd, testConfigLsCmd, testConfigShowCmd, testConfigEditCmd, testConfigDeleteCmd)

	runFlags := func(c *cobra.Command, forEdit bool) {
		c.Flags().StringVar(&trunDesc, "description", "", "description")
		c.Flags().StringVar(&trunStatus, "status", "", "status (active|complete|aborted)")
		c.Flags().StringVar(&trunMilestone, "milestone", "", "milestone slug")
		if forEdit {
			c.Flags().StringVar(&trunTitle, "title", "", "new title")
		}
	}
	runFlags(testRunAddCmd, false)
	runFlags(testRunEditCmd, true)
	testRunConfigCmd.Flags().StringVar(&trunConfig, "config", "", "configuration id")
	testRunUnconfigCmd.Flags().StringVar(&trunConfig, "config", "", "configuration id")
	testRunCmd.AddCommand(testRunAddCmd, testRunLsCmd, testRunShowCmd, testRunEditCmd, testRunDeleteCmd, testRunConfigCmd, testRunUnconfigCmd)

	testResultAddCmd.Flags().StringVar(&tresRun, "run", "", "test run id (required)")
	testResultAddCmd.Flags().StringVar(&tresCase, "case", "", "test case id (required)")
	testResultAddCmd.Flags().StringVar(&tresConfig, "config", "", "configuration id (optional)")
	testResultAddCmd.Flags().StringVar(&tresStatus, "status", "", "outcome (passed|failed|blocked|skipped|invalid|in_progress)")
	testResultAddCmd.Flags().StringVar(&tresComment, "comment", "", "comment")
	testResultAddCmd.Flags().IntVar(&tresDuration, "duration", 0, "duration in ms")
	testResultAddCmd.Flags().StringVar(&tresBy, "by", "", "executed-by (default: actor)")
	_ = testResultAddCmd.MarkFlagRequired("run")
	_ = testResultAddCmd.MarkFlagRequired("case")
	_ = testResultAddCmd.MarkFlagRequired("status")
	testResultCmd.AddCommand(testResultAddCmd, testResultLsCmd)

	testCmd.AddCommand(testConfigCmd, testRunCmd, testResultCmd)
}
