package workspace

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

func TestIsRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"deadlock 1213", &mysql.MySQLError{Number: 1213, Message: "deadlock"}, true},
		{"lock wait 1205", &mysql.MySQLError{Number: 1205, Message: "lock wait timeout"}, true},
		{"other mysql code", &mysql.MySQLError{Number: 1062, Message: "dup key"}, false},
		{"wrapped deadlock", fmt.Errorf("tx: %w", &mysql.MySQLError{Number: 1213}), true},
		{"plain error", errors.New("boom"), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetryable(tc.err); got != tc.want {
				t.Fatalf("isRetryable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestBackoff_ReturnsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already done — backoff should return immediately via the ctx.Done() branch.

	start := time.Now()
	backoff(ctx, 6) // attempt 6 would cap at 2s if it ever waited on the timer.
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("backoff should return promptly on cancelled ctx, took %v", elapsed)
	}
}

func TestBackoff_WaitsForTimer(t *testing.T) {
	// A live (never-cancelled) context takes the timer branch; attempt 0 is ~25ms.
	start := time.Now()
	backoff(context.Background(), 0)
	if elapsed := time.Since(start); elapsed < 10*time.Millisecond {
		t.Fatalf("expected backoff to wait for the timer, took only %v", elapsed)
	}
}
