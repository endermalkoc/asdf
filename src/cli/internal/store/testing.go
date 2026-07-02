package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/endermalkoc/cusp/internal/ids"
)

// Testing layer store — CRUD for the Qase-modeled test tables (test_suite / test_case /
// test_step / test_run / test_result / test_configuration) and their junctions
// (req_requirement_test_case coverage, test_run_configuration). Authored rows are ULID-keyed
// (ids.New); test_result keeps a surrogate id derived deterministically from its identity
// (run_id, test_case_id, configuration_id) so a re-report converges. created_at/updated_at
// carry DB defaults (migration 0015), so inserts omit them.

func nullInt(p *int) any { return nullIfNil(p) }

func scanNullInt(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

// ---- TestSuite ------------------------------------------------------------

type TestSuiteRow struct {
	ID          string `json:"id"`
	ParentID    string `json:"parent_id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Position    *int   `json:"position,omitempty"`
}

func AddTestSuite(ctx context.Context, x Execer, s TestSuiteRow) (TestSuiteRow, error) {
	s.ID = ids.New()
	if _, err := x.ExecContext(ctx,
		"INSERT INTO `test_suite` (id,parent_id,name,description,position) VALUES (?,?,?,?,?)",
		s.ID, nullIfEmpty(s.ParentID), nullIfEmpty(s.Name), nullIfEmpty(s.Description), nullInt(s.Position)); err != nil {
		return TestSuiteRow{}, fmt.Errorf("add test suite %q: %w", s.Name, err)
	}
	return s, nil
}

func ListTestSuites(ctx context.Context, x Execer) ([]TestSuiteRow, error) {
	rows, err := x.QueryContext(ctx,
		"SELECT id, COALESCE(parent_id,''), COALESCE(name,''), COALESCE(description,''), position FROM `test_suite` ORDER BY COALESCE(position,9999), name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TestSuiteRow
	for rows.Next() {
		var s TestSuiteRow
		var pos sql.NullInt64
		if err := rows.Scan(&s.ID, &s.ParentID, &s.Name, &s.Description, &pos); err != nil {
			return nil, err
		}
		s.Position = scanNullInt(pos)
		out = append(out, s)
	}
	return out, rows.Err()
}

func GetTestSuite(ctx context.Context, x Execer, id string) (TestSuiteRow, bool, error) {
	var s TestSuiteRow
	var pos sql.NullInt64
	err := x.QueryRowContext(ctx,
		"SELECT id, COALESCE(parent_id,''), COALESCE(name,''), COALESCE(description,''), position FROM `test_suite` WHERE id=?", id).
		Scan(&s.ID, &s.ParentID, &s.Name, &s.Description, &pos)
	if err == sql.ErrNoRows {
		return TestSuiteRow{}, false, nil
	}
	if err != nil {
		return TestSuiteRow{}, false, err
	}
	s.Position = scanNullInt(pos)
	return s, true, nil
}

func UpdateTestSuite(ctx context.Context, x Execer, s TestSuiteRow) error {
	if _, err := x.ExecContext(ctx,
		"UPDATE `test_suite` SET parent_id=?, name=?, description=?, position=? WHERE id=?",
		nullIfEmpty(s.ParentID), nullIfEmpty(s.Name), nullIfEmpty(s.Description), nullInt(s.Position), s.ID); err != nil {
		return fmt.Errorf("update test suite %s: %w", s.ID, err)
	}
	return nil
}

func DeleteTestSuite(ctx context.Context, x Execer, id string) (bool, error) {
	return deleteByID(ctx, x, "test_suite", id)
}

// ---- TestCase -------------------------------------------------------------

type TestCaseRow struct {
	ID            string `json:"id"`
	SuiteID       string `json:"suite_id"`
	Title         string `json:"title"`
	Description   string `json:"description,omitempty"`
	Preconditions string `json:"preconditions,omitempty"`
	Layer         string `json:"layer,omitempty"`
	Type          string `json:"type,omitempty"`
	Priority      *int   `json:"priority,omitempty"`
	Severity      string `json:"severity,omitempty"`
	Automation    string `json:"automation,omitempty"`
	Status        string `json:"status"`
	Path          string `json:"path,omitempty"`
	IsFlaky       bool   `json:"is_flaky"`
}

const testCaseCols = "id, suite_id, COALESCE(title,''), COALESCE(description,''), COALESCE(preconditions,''), " +
	"COALESCE(layer,''), COALESCE(type,''), priority, COALESCE(severity,''), COALESCE(automation,''), " +
	"status, COALESCE(path,''), is_flaky"

func scanTestCase(s interface{ Scan(...any) error }) (TestCaseRow, error) {
	var c TestCaseRow
	var pri sql.NullInt64
	if err := s.Scan(&c.ID, &c.SuiteID, &c.Title, &c.Description, &c.Preconditions, &c.Layer, &c.Type,
		&pri, &c.Severity, &c.Automation, &c.Status, &c.Path, &c.IsFlaky); err != nil {
		return TestCaseRow{}, err
	}
	c.Priority = scanNullInt(pri)
	return c, nil
}

func AddTestCase(ctx context.Context, x Execer, c TestCaseRow) (TestCaseRow, error) {
	if c.Status == "" {
		c.Status = "draft"
	}
	c.ID = ids.New()
	if _, err := x.ExecContext(ctx,
		"INSERT INTO `test_case` (id,suite_id,title,description,preconditions,layer,type,priority,severity,automation,status,path,is_flaky) "+
			"VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)",
		c.ID, c.SuiteID, nullIfEmpty(c.Title), nullIfEmpty(c.Description), nullIfEmpty(c.Preconditions),
		nullIfEmpty(c.Layer), nullIfEmpty(c.Type), nullInt(c.Priority), nullIfEmpty(c.Severity),
		nullIfEmpty(c.Automation), c.Status, nullIfEmpty(c.Path), c.IsFlaky); err != nil {
		return TestCaseRow{}, fmt.Errorf("add test case %q: %w", c.Title, err)
	}
	return c, nil
}

func ListTestCases(ctx context.Context, x Execer, suiteID string) ([]TestCaseRow, error) {
	q := "SELECT " + testCaseCols + " FROM `test_case`"
	var args []any
	if suiteID != "" {
		q += " WHERE suite_id=?"
		args = append(args, suiteID)
	}
	q += " ORDER BY title"
	rows, err := x.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TestCaseRow
	for rows.Next() {
		c, err := scanTestCase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func GetTestCase(ctx context.Context, x Execer, id string) (TestCaseRow, bool, error) {
	c, err := scanTestCase(x.QueryRowContext(ctx, "SELECT "+testCaseCols+" FROM `test_case` WHERE id=?", id))
	if err == sql.ErrNoRows {
		return TestCaseRow{}, false, nil
	}
	if err != nil {
		return TestCaseRow{}, false, err
	}
	return c, true, nil
}

func UpdateTestCase(ctx context.Context, x Execer, c TestCaseRow) error {
	if _, err := x.ExecContext(ctx,
		"UPDATE `test_case` SET suite_id=?, title=?, description=?, preconditions=?, layer=?, type=?, priority=?, "+
			"severity=?, automation=?, status=?, path=?, is_flaky=? WHERE id=?",
		c.SuiteID, nullIfEmpty(c.Title), nullIfEmpty(c.Description), nullIfEmpty(c.Preconditions),
		nullIfEmpty(c.Layer), nullIfEmpty(c.Type), nullInt(c.Priority), nullIfEmpty(c.Severity),
		nullIfEmpty(c.Automation), c.Status, nullIfEmpty(c.Path), c.IsFlaky, c.ID); err != nil {
		return fmt.Errorf("update test case %s: %w", c.ID, err)
	}
	return nil
}

func DeleteTestCase(ctx context.Context, x Execer, id string) (bool, error) {
	// req_requirement_test_case + test_step cascade; test_result cascades too.
	return deleteByID(ctx, x, "test_case", id)
}

// ---- TestStep -------------------------------------------------------------

type TestStepRow struct {
	ID             string `json:"id"`
	TestCaseID     string `json:"test_case_id"`
	Position       *int   `json:"position,omitempty"`
	Action         string `json:"action,omitempty"`
	ExpectedResult string `json:"expected_result,omitempty"`
}

func AddTestStep(ctx context.Context, x Execer, s TestStepRow) (TestStepRow, error) {
	s.ID = ids.New()
	// position is NOT NULL (an ordinal); when unspecified, append after the case's last step.
	if s.Position == nil {
		var maxPos sql.NullInt64
		if err := x.QueryRowContext(ctx, "SELECT MAX(position) FROM `test_step` WHERE test_case_id=?", s.TestCaseID).Scan(&maxPos); err != nil {
			return TestStepRow{}, err
		}
		next := int(maxPos.Int64) + 1
		s.Position = &next
	}
	if _, err := x.ExecContext(ctx,
		"INSERT INTO `test_step` (id,test_case_id,position,action,expected_result) VALUES (?,?,?,?,?)",
		s.ID, s.TestCaseID, *s.Position, nullIfEmpty(s.Action), nullIfEmpty(s.ExpectedResult)); err != nil {
		return TestStepRow{}, fmt.Errorf("add test step: %w", err)
	}
	return s, nil
}

func ListTestSteps(ctx context.Context, x Execer, testCaseID string) ([]TestStepRow, error) {
	rows, err := x.QueryContext(ctx,
		"SELECT id, test_case_id, position, COALESCE(action,''), COALESCE(expected_result,'') FROM `test_step` WHERE test_case_id=? ORDER BY COALESCE(position,9999)", testCaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TestStepRow
	for rows.Next() {
		var s TestStepRow
		var pos sql.NullInt64
		if err := rows.Scan(&s.ID, &s.TestCaseID, &pos, &s.Action, &s.ExpectedResult); err != nil {
			return nil, err
		}
		s.Position = scanNullInt(pos)
		out = append(out, s)
	}
	return out, rows.Err()
}

func DeleteTestStep(ctx context.Context, x Execer, id string) (bool, error) {
	return deleteByID(ctx, x, "test_step", id)
}

// ---- TestConfiguration ----------------------------------------------------

type ConfigurationRow struct {
	ID          string `json:"id"`
	Group       string `json:"group"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func AddConfiguration(ctx context.Context, x Execer, c ConfigurationRow) (ConfigurationRow, error) {
	c.ID = ids.New()
	if _, err := x.ExecContext(ctx,
		"INSERT INTO `test_configuration` (id,`group`,name,description) VALUES (?,?,?,?)",
		c.ID, c.Group, c.Name, nullIfEmpty(c.Description)); err != nil {
		return ConfigurationRow{}, fmt.Errorf("add configuration %s/%s: %w", c.Group, c.Name, err)
	}
	return c, nil
}

func ListConfigurations(ctx context.Context, x Execer) ([]ConfigurationRow, error) {
	rows, err := x.QueryContext(ctx,
		"SELECT id, `group`, name, COALESCE(description,'') FROM `test_configuration` ORDER BY `group`, name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConfigurationRow
	for rows.Next() {
		var c ConfigurationRow
		if err := rows.Scan(&c.ID, &c.Group, &c.Name, &c.Description); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func GetConfiguration(ctx context.Context, x Execer, id string) (ConfigurationRow, bool, error) {
	var c ConfigurationRow
	err := x.QueryRowContext(ctx,
		"SELECT id, `group`, name, COALESCE(description,'') FROM `test_configuration` WHERE id=?", id).
		Scan(&c.ID, &c.Group, &c.Name, &c.Description)
	if err == sql.ErrNoRows {
		return ConfigurationRow{}, false, nil
	}
	if err != nil {
		return ConfigurationRow{}, false, err
	}
	return c, true, nil
}

func UpdateConfiguration(ctx context.Context, x Execer, c ConfigurationRow) error {
	if _, err := x.ExecContext(ctx,
		"UPDATE `test_configuration` SET `group`=?, name=?, description=? WHERE id=?",
		c.Group, c.Name, nullIfEmpty(c.Description), c.ID); err != nil {
		return fmt.Errorf("update configuration %s: %w", c.ID, err)
	}
	return nil
}

func DeleteConfiguration(ctx context.Context, x Execer, id string) (bool, error) {
	return deleteByID(ctx, x, "test_configuration", id)
}

// ---- TestRun --------------------------------------------------------------

type TestRunRow struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Description   string `json:"description,omitempty"`
	Status        string `json:"status"`
	MilestoneSlug string `json:"milestone,omitempty"`
}

func AddTestRun(ctx context.Context, x Execer, r TestRunRow, milestoneID string) (TestRunRow, error) {
	if r.Status == "" {
		r.Status = "active"
	}
	r.ID = ids.New()
	if _, err := x.ExecContext(ctx,
		"INSERT INTO `test_run` (id,title,description,status,milestone_id,started_at) VALUES (?,?,?,?,?,?)",
		r.ID, nullIfEmpty(r.Title), nullIfEmpty(r.Description), r.Status, nullIfEmpty(milestoneID), time.Now().UTC()); err != nil {
		return TestRunRow{}, fmt.Errorf("add test run %q: %w", r.Title, err)
	}
	return r, nil
}

func ListTestRuns(ctx context.Context, x Execer) ([]TestRunRow, error) {
	rows, err := x.QueryContext(ctx,
		"SELECT r.id, COALESCE(r.title,''), COALESCE(r.description,''), r.status, COALESCE(m.slug,'') "+
			"FROM `test_run` r LEFT JOIN `plan_milestone` m ON r.milestone_id=m.id ORDER BY r.started_at DESC, r.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TestRunRow
	for rows.Next() {
		var r TestRunRow
		if err := rows.Scan(&r.ID, &r.Title, &r.Description, &r.Status, &r.MilestoneSlug); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func GetTestRun(ctx context.Context, x Execer, id string) (TestRunRow, bool, error) {
	var r TestRunRow
	err := x.QueryRowContext(ctx,
		"SELECT r.id, COALESCE(r.title,''), COALESCE(r.description,''), r.status, COALESCE(m.slug,'') "+
			"FROM `test_run` r LEFT JOIN `plan_milestone` m ON r.milestone_id=m.id WHERE r.id=?", id).
		Scan(&r.ID, &r.Title, &r.Description, &r.Status, &r.MilestoneSlug)
	if err == sql.ErrNoRows {
		return TestRunRow{}, false, nil
	}
	if err != nil {
		return TestRunRow{}, false, err
	}
	return r, true, nil
}

// UpdateTestRun writes title/description/status; when status leaves 'active', ended_at is stamped.
func UpdateTestRun(ctx context.Context, x Execer, r TestRunRow, milestoneID string) error {
	var ended any
	if r.Status == "complete" || r.Status == "aborted" {
		ended = time.Now().UTC()
	}
	if _, err := x.ExecContext(ctx,
		"UPDATE `test_run` SET title=?, description=?, status=?, milestone_id=?, ended_at=COALESCE(?, ended_at) WHERE id=?",
		nullIfEmpty(r.Title), nullIfEmpty(r.Description), r.Status, nullIfEmpty(milestoneID), ended, r.ID); err != nil {
		return fmt.Errorf("update test run %s: %w", r.ID, err)
	}
	return nil
}

func DeleteTestRun(ctx context.Context, x Execer, id string) (bool, error) {
	return deleteByID(ctx, x, "test_run", id)
}

// ---- TestResult (deterministic id) ----------------------------------------

type TestResultRow struct {
	ID              string `json:"id"`
	RunID           string `json:"run_id"`
	TestCaseID      string `json:"test_case_id"`
	ConfigurationID string `json:"configuration_id,omitempty"`
	Status          string `json:"status,omitempty"`
	Comment         string `json:"comment,omitempty"`
	DurationMs      *int   `json:"duration_ms,omitempty"`
	ExecutedBy      string `json:"executed_by,omitempty"`
}

// UpsertTestResult records a result; the id is deterministic over (run, case, configuration) so a
// re-report of the same execution converges instead of duplicating (INSERT … ON DUPLICATE KEY UPDATE).
func UpsertTestResult(ctx context.Context, x Execer, r TestResultRow) (TestResultRow, error) {
	r.ID = ids.Rel("test_result", r.RunID, r.TestCaseID, r.ConfigurationID)
	now := time.Now().UTC()
	if _, err := x.ExecContext(ctx,
		"INSERT INTO `test_result` (id,run_id,test_case_id,configuration_id,status,comment,duration_ms,executed_by,executed_at) "+
			"VALUES (?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE status=?, comment=?, duration_ms=?, executed_by=?, executed_at=?",
		r.ID, r.RunID, r.TestCaseID, nullIfEmpty(r.ConfigurationID), nullIfEmpty(r.Status), nullIfEmpty(r.Comment),
		nullInt(r.DurationMs), nullIfEmpty(r.ExecutedBy), now,
		nullIfEmpty(r.Status), nullIfEmpty(r.Comment), nullInt(r.DurationMs), nullIfEmpty(r.ExecutedBy), now); err != nil {
		return TestResultRow{}, fmt.Errorf("record test result: %w", err)
	}
	return r, nil
}

func ListTestResults(ctx context.Context, x Execer, runID string) ([]TestResultRow, error) {
	rows, err := x.QueryContext(ctx,
		"SELECT id, run_id, test_case_id, COALESCE(configuration_id,''), COALESCE(status,''), COALESCE(comment,''), duration_ms, COALESCE(executed_by,'') "+
			"FROM `test_result` WHERE run_id=? ORDER BY id", runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TestResultRow
	for rows.Next() {
		var r TestResultRow
		var dur sql.NullInt64
		if err := rows.Scan(&r.ID, &r.RunID, &r.TestCaseID, &r.ConfigurationID, &r.Status, &r.Comment, &dur, &r.ExecutedBy); err != nil {
			return nil, err
		}
		r.DurationMs = scanNullInt(dur)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- junctions ------------------------------------------------------------

// LinkRequirementTestCase / UnlinkRequirementTestCase — req_requirement_test_case coverage.
func LinkRequirementTestCase(ctx context.Context, x Execer, requirementID, testCaseID string) error {
	return linkJunction(ctx, x, "req_requirement_test_case", "requirement_id", "test_case_id", requirementID, testCaseID)
}
func UnlinkRequirementTestCase(ctx context.Context, x Execer, requirementID, testCaseID string) error {
	return unlinkJunction(ctx, x, "req_requirement_test_case", "requirement_id", "test_case_id", requirementID, testCaseID)
}

// LinkRunConfiguration / UnlinkRunConfiguration — test_run_configuration.
func LinkRunConfiguration(ctx context.Context, x Execer, runID, configurationID string) error {
	return linkJunction(ctx, x, "test_run_configuration", "run_id", "configuration_id", runID, configurationID)
}
func UnlinkRunConfiguration(ctx context.Context, x Execer, runID, configurationID string) error {
	return unlinkJunction(ctx, x, "test_run_configuration", "run_id", "configuration_id", runID, configurationID)
}

// ---- deterministic upserts (for import: caller supplies a stable id) ------
//
// The Add* funcs above mint a fresh ULID each call (for authored CLI rows). Import needs
// convergence on re-run, so these take a caller-supplied deterministic id (ids.Rel over the
// source id) and INSERT-or-UPDATE by it — mirroring the planning/requirements upserts. Each
// returns whether it inserted (true) or updated.

func upsertByID(ctx context.Context, x Execer, table, id string, insert, update func() error) (bool, error) {
	var existing string
	err := x.QueryRowContext(ctx, "SELECT id FROM `"+table+"` WHERE id=?", id).Scan(&existing)
	switch {
	case err == sql.ErrNoRows:
		return true, insert()
	case err != nil:
		return false, err
	default:
		return false, update()
	}
}

// UpsertTestSuite upserts a suite by id; parent is set separately (SetTestSuiteParent) so the
// self-referencing FK always resolves.
func UpsertTestSuite(ctx context.Context, x Execer, s TestSuiteRow) (bool, error) {
	return upsertByID(ctx, x, "test_suite", s.ID, func() error {
		_, err := x.ExecContext(ctx, "INSERT INTO `test_suite` (id,name,description,position) VALUES (?,?,?,?)",
			s.ID, nullIfEmpty(s.Name), nullIfEmpty(s.Description), nullInt(s.Position))
		return err
	}, func() error {
		_, err := x.ExecContext(ctx, "UPDATE `test_suite` SET name=?, description=?, position=? WHERE id=?",
			nullIfEmpty(s.Name), nullIfEmpty(s.Description), nullInt(s.Position), s.ID)
		return err
	})
}

// SetTestSuiteParent sets (or clears) a suite's parent_id; run after all suites exist.
func SetTestSuiteParent(ctx context.Context, x Execer, id, parentID string) error {
	if _, err := x.ExecContext(ctx, "UPDATE `test_suite` SET parent_id=? WHERE id=?", nullIfEmpty(parentID), id); err != nil {
		return fmt.Errorf("set suite %s parent: %w", id, err)
	}
	return nil
}

// UpsertTestCase upserts a test case by id.
func UpsertTestCase(ctx context.Context, x Execer, c TestCaseRow) (bool, error) {
	if c.Status == "" {
		c.Status = "draft"
	}
	return upsertByID(ctx, x, "test_case", c.ID, func() error {
		_, err := x.ExecContext(ctx,
			"INSERT INTO `test_case` (id,suite_id,title,description,preconditions,layer,type,priority,severity,automation,status,path,is_flaky) "+
				"VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)",
			c.ID, c.SuiteID, nullIfEmpty(c.Title), nullIfEmpty(c.Description), nullIfEmpty(c.Preconditions),
			nullIfEmpty(c.Layer), nullIfEmpty(c.Type), nullInt(c.Priority), nullIfEmpty(c.Severity),
			nullIfEmpty(c.Automation), c.Status, nullIfEmpty(c.Path), c.IsFlaky)
		return err
	}, func() error {
		return UpdateTestCase(ctx, x, c)
	})
}

// UpsertTestStep upserts a step by id (position is NOT NULL; defaults to 0 when unset).
func UpsertTestStep(ctx context.Context, x Execer, s TestStepRow) (bool, error) {
	pos := 0
	if s.Position != nil {
		pos = *s.Position
	}
	return upsertByID(ctx, x, "test_step", s.ID, func() error {
		_, err := x.ExecContext(ctx, "INSERT INTO `test_step` (id,test_case_id,position,action,expected_result) VALUES (?,?,?,?,?)",
			s.ID, s.TestCaseID, pos, nullIfEmpty(s.Action), nullIfEmpty(s.ExpectedResult))
		return err
	}, func() error {
		_, err := x.ExecContext(ctx, "UPDATE `test_step` SET position=?, action=?, expected_result=? WHERE id=?",
			pos, nullIfEmpty(s.Action), nullIfEmpty(s.ExpectedResult), s.ID)
		return err
	})
}

// UpsertConfiguration upserts a configuration by id.
func UpsertConfiguration(ctx context.Context, x Execer, c ConfigurationRow) (bool, error) {
	return upsertByID(ctx, x, "test_configuration", c.ID, func() error {
		_, err := x.ExecContext(ctx, "INSERT INTO `test_configuration` (id,`group`,name,description) VALUES (?,?,?,?)",
			c.ID, c.Group, c.Name, nullIfEmpty(c.Description))
		return err
	}, func() error {
		return UpdateConfiguration(ctx, x, c)
	})
}

// UpsertTestRun upserts a run by id.
func UpsertTestRun(ctx context.Context, x Execer, r TestRunRow, milestoneID string) (bool, error) {
	if r.Status == "" {
		r.Status = "active"
	}
	return upsertByID(ctx, x, "test_run", r.ID, func() error {
		_, err := x.ExecContext(ctx,
			"INSERT INTO `test_run` (id,title,description,status,milestone_id,started_at) VALUES (?,?,?,?,?,?)",
			r.ID, nullIfEmpty(r.Title), nullIfEmpty(r.Description), r.Status, nullIfEmpty(milestoneID), time.Now().UTC())
		return err
	}, func() error {
		return UpdateTestRun(ctx, x, r, milestoneID)
	})
}

// deleteByID is a shared "DELETE FROM <table> WHERE id=?" returning whether a row was removed.
func deleteByID(ctx context.Context, x Execer, table, id string) (bool, error) {
	res, err := x.ExecContext(ctx, "DELETE FROM `"+table+"` WHERE id=?", id)
	if err != nil {
		return false, fmt.Errorf("delete %s %s: %w", table, id, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
