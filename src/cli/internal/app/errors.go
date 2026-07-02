package app

import "fmt"

// Exit codes for categorized CLI failures (documented in docs/command-contract.md). Any
// error that is not a *CodedError maps to ExitGeneric.
const (
	ExitGeneric    = 1
	ExitValidation = 2
	ExitNotFound   = 3
	ExitDangling   = 4
)

// SilentExit signals a nonzero exit WITHOUT printing an error message or a --json error envelope.
// It is for commands that already printed their own report and still want to gate via the exit
// code (e.g. `cusp doctor` when problems are found) — the report is the sole output.
type SilentExit struct{ Code int }

func (e *SilentExit) Error() string { return "" }

// ExitWith returns a SilentExit for a nonzero code (a 0 code is success → nil).
func ExitWith(code int) error {
	if code == 0 {
		return nil
	}
	return &SilentExit{Code: code}
}

// CodedError carries an exit code and a machine-readable category for a failure, so the
// CLI can map it to a documented exit code and a --json error envelope. Unwrap exposes
// the underlying cause for errors.As/Is.
type CodedError struct {
	Code     int
	Category string
	Err      error
}

func (e *CodedError) Error() string { return e.Err.Error() }
func (e *CodedError) Unwrap() error { return e.Err }

func coded(code int, category string, err error) error {
	if err == nil {
		return nil
	}
	return &CodedError{Code: code, Category: category, Err: err}
}

// ValidationFailed tags err as a validation failure (exit 2).
func ValidationFailed(err error) error { return coded(ExitValidation, "validation", err) }

// NotFound builds a `no <kind> "<key>"` not-found error (exit 3).
func NotFound(kind, key string) error {
	return coded(ExitNotFound, "not_found", fmt.Errorf("no %s %q", kind, key))
}

// NotFoundErr tags an existing error as not-found (exit 3) — for custom messages.
func NotFoundErr(err error) error { return coded(ExitNotFound, "not_found", err) }
