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

var testCmd = &cobra.Command{Use: "test", Short: "Manage the testing layer (suites, cases, steps, runs, results, configs)"}

// intFlagArg returns a *int only when the named flag was passed (so an unset nullable column
// stays NULL rather than becoming 0).
func intFlagArg(cmd *cobra.Command, name string, v int) *int {
	if cmd.Flags().Changed(name) {
		return &v
	}
	return nil
}

// ---- test suite -----------------------------------------------------------

var (
	tsuiteParent string
	tsuiteDesc   string
	tsuitePos    int
	tsuiteName   string // edit
)

var testSuiteCmd = &cobra.Command{Use: "suite", Short: "Manage test suites (a self-nesting tree)"}

var testSuiteAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a test suite",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleMutate(cmd, "add test suite "+args[0], func(vctx context.Context, r store.Execer) error {
			return validateSuiteParent(vctx, r, tsuiteParent)
		}, func(ctx context.Context, w *app.Write) (any, string, error) {
			s, e := store.AddTestSuite(ctx, w.Tx, store.TestSuiteRow{
				ParentID: tsuiteParent, Name: args[0], Description: tsuiteDesc, Position: intFlagArg(cmd, "position", tsuitePos),
			})
			if e != nil {
				return nil, "", e
			}
			w.MarkDirty("test_suite")
			return s, fmt.Sprintf("added test suite %q  (id=%s)", s.Name, s.ID), nil
		})
	},
}

func validateSuiteParent(ctx context.Context, r store.Execer, parent string) error {
	if parent == "" {
		return nil
	}
	if _, ok, e := store.GetTestSuite(ctx, r, parent); e != nil {
		return e
	} else if !ok {
		return app.NotFound("parent suite", parent)
	}
	return nil
}

var testSuiteLsCmd = &cobra.Command{
	Use: "ls", Short: "List test suites", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleList(cmd, func(ctx context.Context, r store.Execer) (any, string, error) {
			suites, e := store.ListTestSuites(ctx, r)
			if e != nil {
				return nil, "", e
			}
			var b strings.Builder
			for _, s := range suites {
				fmt.Fprintf(&b, "%-26s %s\n", s.ID, s.Name)
			}
			return suites, b.String(), nil
		})
	},
}

var testSuiteShowCmd = &cobra.Command{
	Use: "show <id>", Short: "Show a test suite", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleGet(cmd, store.GetTestSuite, "test suite", args[0])
	},
}

var testSuiteEditCmd = &cobra.Command{
	Use: "edit <id>", Short: "Edit a test suite (only the flags you pass change)", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().NFlag() == 0 {
			return fmt.Errorf("nothing to edit — pass --name/--description/--parent/--position")
		}
		var s store.TestSuiteRow
		return simpleMutate(cmd, "edit test suite "+args[0], func(vctx context.Context, r store.Execer) error {
			cur, ok, e := store.GetTestSuite(vctx, r, args[0])
			if e != nil {
				return e
			}
			if !ok {
				return app.NotFound("test suite", args[0])
			}
			if cmd.Flags().Changed("name") {
				cur.Name = tsuiteName
			}
			if cmd.Flags().Changed("description") {
				cur.Description = tsuiteDesc
			}
			if cmd.Flags().Changed("position") {
				cur.Position = intFlagArg(cmd, "position", tsuitePos)
			}
			if cmd.Flags().Changed("parent") {
				if e := validateSuiteParent(vctx, r, tsuiteParent); e != nil {
					return e
				}
				cur.ParentID = tsuiteParent
			}
			s = cur
			return nil
		}, func(ctx context.Context, w *app.Write) (any, string, error) {
			if e := store.UpdateTestSuite(ctx, w.Tx, s); e != nil {
				return nil, "", e
			}
			w.MarkDirty("test_suite")
			return s, "updated test suite " + s.ID, nil
		})
	},
}

var testSuiteDeleteCmd = &cobra.Command{
	Use: "delete <id>", Short: "Delete a test suite (its cases cascade)", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleDelete(cmd, "delete test suite "+args[0], "test suite", args[0],
			store.DeleteTestSuite, "test_suite", "test_case", "test_step", "req_requirement_test_case", "test_result")
	},
}

// ---- test case ------------------------------------------------------------

var (
	tcaseSuite      string
	tcaseDesc       string
	tcasePre        string
	tcaseLayer      string
	tcaseType       string
	tcasePriority   int
	tcaseSeverity   string
	tcaseAutomation string
	tcaseStatus     string
	tcasePath       string
	tcaseFlaky      bool
	tcaseTitle      string // edit
	tcaseReq        string // cover/uncover
)

var testCaseCmd = &cobra.Command{Use: "case", Short: "Manage test cases"}

func validateTestCaseEnums(cmd *cobra.Command, requireAll bool) error {
	checks := []struct {
		flag, field, val string
		set              []string
	}{
		{"layer", "layer", tcaseLayer, enums.TestLayer},
		{"type", "type", tcaseType, enums.TestType},
		{"severity", "severity", tcaseSeverity, enums.TestSeverity},
		{"automation", "automation", tcaseAutomation, enums.TestAutomation},
		{"status", "status", tcaseStatus, enums.TestCaseStatus},
	}
	for _, c := range checks {
		if requireAll || cmd.Flags().Changed(c.flag) {
			if e := app.ValidateEnum(c.field, c.val, c.set); e != nil {
				return e
			}
		}
	}
	if cmd.Flags().Changed("priority") && (tcasePriority < 0 || tcasePriority > 4) {
		return app.ValidationFailed(fmt.Errorf("priority must be 0–4"))
	}
	return nil
}

var testCaseAddCmd = &cobra.Command{
	Use: "add <title>", Short: "Add a test case to a suite", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleMutate(cmd, "add test case "+args[0], func(vctx context.Context, r store.Execer) error {
			if e := validateTestCaseEnums(cmd, false); e != nil {
				return e
			}
			if _, ok, e := store.GetTestSuite(vctx, r, tcaseSuite); e != nil {
				return e
			} else if !ok {
				return app.NotFound("suite", tcaseSuite)
			}
			return nil
		}, func(ctx context.Context, w *app.Write) (any, string, error) {
			c, e := store.AddTestCase(ctx, w.Tx, store.TestCaseRow{
				SuiteID: tcaseSuite, Title: args[0], Description: tcaseDesc, Preconditions: tcasePre,
				Layer: tcaseLayer, Type: tcaseType, Priority: intFlagArg(cmd, "priority", tcasePriority),
				Severity: tcaseSeverity, Automation: tcaseAutomation, Status: tcaseStatus, Path: tcasePath, IsFlaky: tcaseFlaky,
			})
			if e != nil {
				return nil, "", e
			}
			w.MarkDirty("test_case")
			return c, fmt.Sprintf("added test case %q  (id=%s)", c.Title, c.ID), nil
		})
	},
}

var testCaseLsCmd = &cobra.Command{
	Use: "ls [suite-id]", Short: "List test cases (optionally within a suite)", Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		suite := ""
		if len(args) > 0 {
			suite = args[0]
		}
		return simpleList(cmd, func(ctx context.Context, r store.Execer) (any, string, error) {
			cases, e := store.ListTestCases(ctx, r, suite)
			if e != nil {
				return nil, "", e
			}
			var b strings.Builder
			for _, c := range cases {
				fmt.Fprintf(&b, "%-26s %-10s %-8s %s\n", c.ID, c.Status, c.Layer, c.Title)
			}
			return cases, b.String(), nil
		})
	},
}

var testCaseShowCmd = &cobra.Command{
	Use: "show <id>", Short: "Show a test case", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleGet(cmd, store.GetTestCase, "test case", args[0])
	},
}

var testCaseEditCmd = &cobra.Command{
	Use: "edit <id>", Short: "Edit a test case (only the flags you pass change)", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().NFlag() == 0 {
			return fmt.Errorf("nothing to edit — pass one of --title/--suite/--description/--preconditions/--layer/--type/--priority/--severity/--automation/--status/--path/--flaky")
		}
		var c store.TestCaseRow
		return simpleMutate(cmd, "edit test case "+args[0], func(vctx context.Context, r store.Execer) error {
			cur, ok, e := store.GetTestCase(vctx, r, args[0])
			if e != nil {
				return e
			}
			if !ok {
				return app.NotFound("test case", args[0])
			}
			if e := validateTestCaseEnums(cmd, false); e != nil {
				return e
			}
			if cmd.Flags().Changed("suite") {
				if _, ok, e := store.GetTestSuite(vctx, r, tcaseSuite); e != nil {
					return e
				} else if !ok {
					return app.NotFound("suite", tcaseSuite)
				}
				cur.SuiteID = tcaseSuite
			}
			applyStr(cmd, "title", tcaseTitle, &cur.Title)
			applyStr(cmd, "description", tcaseDesc, &cur.Description)
			applyStr(cmd, "preconditions", tcasePre, &cur.Preconditions)
			applyStr(cmd, "layer", tcaseLayer, &cur.Layer)
			applyStr(cmd, "type", tcaseType, &cur.Type)
			applyStr(cmd, "severity", tcaseSeverity, &cur.Severity)
			applyStr(cmd, "automation", tcaseAutomation, &cur.Automation)
			applyStr(cmd, "status", tcaseStatus, &cur.Status)
			applyStr(cmd, "path", tcasePath, &cur.Path)
			if cmd.Flags().Changed("priority") {
				cur.Priority = intFlagArg(cmd, "priority", tcasePriority)
			}
			if cmd.Flags().Changed("flaky") {
				cur.IsFlaky = tcaseFlaky
			}
			c = cur
			return nil
		}, func(ctx context.Context, w *app.Write) (any, string, error) {
			if e := store.UpdateTestCase(ctx, w.Tx, c); e != nil {
				return nil, "", e
			}
			w.MarkDirty("test_case")
			return c, "updated test case " + c.ID, nil
		})
	},
}

var testCaseDeleteCmd = &cobra.Command{
	Use: "delete <id>", Short: "Delete a test case (steps, coverage, results cascade)", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleDelete(cmd, "delete test case "+args[0], "test case", args[0],
			store.DeleteTestCase, "test_case", "test_step", "req_requirement_test_case", "test_result")
	},
}

var testCaseCoverCmd = &cobra.Command{
	Use: "cover <case-id>", Short: "Link a test case to a requirement it covers (--req <fr_key>)", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error { return testCaseCover(cmd, args[0], false) },
}
var testCaseUncoverCmd = &cobra.Command{
	Use: "uncover <case-id>", Short: "Remove a test case's coverage of a requirement (--req <fr_key>)", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error { return testCaseCover(cmd, args[0], true) },
}

func testCaseCover(cmd *cobra.Command, caseID string, remove bool) error {
	if tcaseReq == "" {
		return app.ValidationFailed(fmt.Errorf("pass --req <fr_key>"))
	}
	verb := "cover"
	if remove {
		verb = "uncover"
	}
	var reqID string
	return simpleMutate(cmd, verb+" test case "+caseID, func(vctx context.Context, r store.Execer) error {
		if _, ok, e := store.GetTestCase(vctx, r, caseID); e != nil {
			return e
		} else if !ok {
			return app.NotFound("test case", caseID)
		}
		req, ok, e := store.GetRequirement(vctx, r, tcaseReq)
		if e != nil {
			return e
		}
		if !ok {
			return app.NotFound("requirement", tcaseReq)
		}
		reqID = req.ID
		return nil
	}, func(ctx context.Context, w *app.Write) (any, string, error) {
		var e error
		if remove {
			e = store.UnlinkRequirementTestCase(ctx, w.Tx, reqID, caseID)
		} else {
			e = store.LinkRequirementTestCase(ctx, w.Tx, reqID, caseID)
		}
		if e != nil {
			return nil, "", e
		}
		w.MarkDirty("req_requirement_test_case")
		return map[string]any{"testCase": caseID, "requirement": tcaseReq, "removed": remove},
			fmt.Sprintf("%sed %s ↔ %s", verb, caseID, tcaseReq), nil
	})
}

// ---- test case steps ------------------------------------------------------

var (
	tstepAction   string
	tstepExpected string
	tstepPos      int
)

var testStepCmd = &cobra.Command{Use: "step", Short: "Manage a test case's steps"}

var testStepAddCmd = &cobra.Command{
	Use: "add <case-id>", Short: "Add a step to a test case", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleMutate(cmd, "add test step to "+args[0], func(vctx context.Context, r store.Execer) error {
			if _, ok, e := store.GetTestCase(vctx, r, args[0]); e != nil {
				return e
			} else if !ok {
				return app.NotFound("test case", args[0])
			}
			return nil
		}, func(ctx context.Context, w *app.Write) (any, string, error) {
			s, e := store.AddTestStep(ctx, w.Tx, store.TestStepRow{
				TestCaseID: args[0], Action: tstepAction, ExpectedResult: tstepExpected, Position: intFlagArg(cmd, "position", tstepPos),
			})
			if e != nil {
				return nil, "", e
			}
			w.MarkDirty("test_step")
			return s, "added step (id=" + s.ID + ")", nil
		})
	},
}

var testStepLsCmd = &cobra.Command{
	Use: "ls <case-id>", Short: "List a test case's steps", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleList(cmd, func(ctx context.Context, r store.Execer) (any, string, error) {
			steps, e := store.ListTestSteps(ctx, r, args[0])
			if e != nil {
				return nil, "", e
			}
			var b strings.Builder
			for i, s := range steps {
				fmt.Fprintf(&b, "%d. %s → %s  (%s)\n", i+1, oneLineComment(s.Action), oneLineComment(s.ExpectedResult), s.ID)
			}
			return steps, b.String(), nil
		})
	},
}

var testStepDeleteCmd = &cobra.Command{
	Use: "delete <step-id>", Short: "Delete a test step", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return simpleDelete(cmd, "delete test step "+args[0], "test step", args[0], store.DeleteTestStep, "test_step")
	},
}

func init() {
	testSuiteAddCmd.Flags().StringVar(&tsuiteParent, "parent", "", "parent suite id")
	testSuiteAddCmd.Flags().StringVar(&tsuiteDesc, "description", "", "description")
	testSuiteAddCmd.Flags().IntVar(&tsuitePos, "position", 0, "sibling order")
	testSuiteEditCmd.Flags().StringVar(&tsuiteName, "name", "", "new name")
	testSuiteEditCmd.Flags().StringVar(&tsuiteParent, "parent", "", "parent suite id (empty to clear)")
	testSuiteEditCmd.Flags().StringVar(&tsuiteDesc, "description", "", "description")
	testSuiteEditCmd.Flags().IntVar(&tsuitePos, "position", 0, "sibling order")
	testSuiteCmd.AddCommand(testSuiteAddCmd, testSuiteLsCmd, testSuiteShowCmd, testSuiteEditCmd, testSuiteDeleteCmd)

	caseFlags := func(c *cobra.Command, forEdit bool) {
		c.Flags().StringVar(&tcaseSuite, "suite", "", "owning suite id")
		c.Flags().StringVar(&tcaseDesc, "description", "", "description")
		c.Flags().StringVar(&tcasePre, "preconditions", "", "preconditions")
		c.Flags().StringVar(&tcaseLayer, "layer", "", "layer (unit|integration|e2e|component|shared)")
		c.Flags().StringVar(&tcaseType, "type", "", "type (functional|smoke|regression|acceptance|other)")
		c.Flags().IntVar(&tcasePriority, "priority", 0, "priority 0–4")
		c.Flags().StringVar(&tcaseSeverity, "severity", "", "severity (trivial|minor|normal|major|critical|blocker)")
		c.Flags().StringVar(&tcaseAutomation, "automation", "", "automation (manual|automated|to_be_automated)")
		c.Flags().StringVar(&tcaseStatus, "status", "", "status (draft|active|deprecated)")
		c.Flags().StringVar(&tcasePath, "path", "", "automated test path")
		c.Flags().BoolVar(&tcaseFlaky, "flaky", false, "mark as flaky")
		if forEdit {
			c.Flags().StringVar(&tcaseTitle, "title", "", "new title")
		}
	}
	caseFlags(testCaseAddCmd, false)
	_ = testCaseAddCmd.MarkFlagRequired("suite")
	caseFlags(testCaseEditCmd, true)
	testCaseCoverCmd.Flags().StringVar(&tcaseReq, "req", "", "requirement fr_key")
	testCaseUncoverCmd.Flags().StringVar(&tcaseReq, "req", "", "requirement fr_key")
	testStepAddCmd.Flags().StringVar(&tstepAction, "action", "", "step action")
	testStepAddCmd.Flags().StringVar(&tstepExpected, "expected", "", "expected result")
	testStepAddCmd.Flags().IntVar(&tstepPos, "position", 0, "step order")
	testStepCmd.AddCommand(testStepAddCmd, testStepLsCmd, testStepDeleteCmd)
	testCaseCmd.AddCommand(testCaseAddCmd, testCaseLsCmd, testCaseShowCmd, testCaseEditCmd, testCaseDeleteCmd, testCaseCoverCmd, testCaseUncoverCmd, testStepCmd)

	testCmd.AddCommand(testSuiteCmd, testCaseCmd)
	rootCmd.AddCommand(testCmd)
}
