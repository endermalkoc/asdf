package store

import (
	"testing"
	"time"
)

// These tests exercise the pure, DB-free helpers (nullable-column mappers, the
// doc-path reconstruction, and the kebab/dir helpers). They live in the internal
// test package so they can reach the unexported helpers; they must not import
// internal/app or internal/testutil (that would be an import cycle).

func TestNullIfEmpty(t *testing.T) {
	if got := nullIfEmpty(""); got != nil {
		t.Errorf("nullIfEmpty(%q) = %v, want nil", "", got)
	}
	if got := nullIfEmpty("x"); got != "x" {
		t.Errorf("nullIfEmpty(%q) = %v, want %q", "x", got, "x")
	}
}

func TestNullIfNil(t *testing.T) {
	if got := nullIfNil(nil); got != nil {
		t.Errorf("nullIfNil(nil) = %v, want nil", got)
	}
	v := 7
	if got := nullIfNil(&v); got != 7 {
		t.Errorf("nullIfNil(&7) = %v, want 7", got)
	}
}

func TestIsoDateArg(t *testing.T) {
	if got := isoDateArg(""); got != nil {
		t.Errorf("isoDateArg(%q) = %v, want nil", "", got)
	}
	if got := isoDateArg("not-a-date"); got != nil {
		t.Errorf("isoDateArg(%q) = %v, want nil (unparseable)", "not-a-date", got)
	}
	got := isoDateArg("2024-01-15")
	tm, ok := got.(time.Time)
	if !ok {
		t.Fatalf("isoDateArg(valid) = %T, want time.Time", got)
	}
	if tm.Year() != 2024 || tm.Month() != time.January || tm.Day() != 15 {
		t.Errorf("isoDateArg parsed to %v, want 2024-01-15", tm)
	}
	if tm.Location() != time.UTC {
		t.Errorf("isoDateArg location = %v, want UTC", tm.Location())
	}
}

func TestKebabName(t *testing.T) {
	cases := map[string]string{
		"EventAttendance": "event-attendance",
		"Group Tag":       "group-tag",
		"Student":         "student",
		"already-kebab":   "already-kebab",
		"HTTP Server":     "http-server",
	}
	for in, want := range cases {
		if got := kebabName(in); got != want {
			t.Errorf("kebabName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDirPrefix(t *testing.T) {
	if got := dirPrefix(""); got != "" {
		t.Errorf("dirPrefix(%q) = %q, want %q", "", got, "")
	}
	if got := dirPrefix("events"); got != "events/" {
		t.Errorf("dirPrefix(%q) = %q, want %q", "events", got, "events/")
	}
}

func TestSpecDocPath(t *testing.T) {
	if got := SpecDocPath("scheduling", "events", "take-attendance"); got != "scheduling/events/take-attendance.md" {
		t.Errorf("SpecDocPath = %q", got)
	}
	if got := SpecDocPath("scheduling", "", "overview"); got != "scheduling/overview.md" {
		t.Errorf("SpecDocPath (no dir) = %q", got)
	}
}

func TestEntityDocPath(t *testing.T) {
	if got := EntityDocPath("", "EventAttendance"); got != "entities/event-attendance.md" {
		t.Errorf("EntityDocPath = %q", got)
	}
	if got := EntityDocPath("sub", "Group Tag"); got != "entities/sub/group-tag.md" {
		t.Errorf("EntityDocPath (subdir) = %q", got)
	}
}
