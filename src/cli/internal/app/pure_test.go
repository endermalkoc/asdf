package app

import (
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/endermalkoc/cusp/internal/refs"
	"github.com/endermalkoc/cusp/internal/store"
)

// --- read.go pure helpers ---------------------------------------------------

func TestValidRef(t *testing.T) {
	cases := []struct {
		ref  string
		want bool
	}{
		{"main", true},
		{"changeset/foo-bar_1", true},
		{"0123abcDEF", true},
		{"HEAD", true},
		{"", false},
		{"has space", false},
		{"semi;colon", false},
		{"quote'inject", false},
		{"back`tick", false},
		{"tilde~1", false}, // validRef excludes ~ (that is quoteRef's set, not this one)
	}
	for _, c := range cases {
		if got := validRef(c.ref); got != c.want {
			t.Errorf("validRef(%q)=%v want %v", c.ref, got, c.want)
		}
	}
}

func TestBaseDB(t *testing.T) {
	cases := map[string]string{
		"cusp":            "cusp",
		"cusp/main":       "cusp",
		"cusp/abc123hash": "cusp",
		"":                "",
	}
	for in, want := range cases {
		if got := baseDB(in); got != want {
			t.Errorf("baseDB(%q)=%q want %q", in, got, want)
		}
	}
}

// --- autogen.go pure helpers ------------------------------------------------

func TestQuoteRef(t *testing.T) {
	cases := []struct {
		ref  string
		want string
	}{
		{"abc123", "'abc123'"},
		{"HEAD~1", "'HEAD~1'"},
		{"main^", "'main^'"},
		{"changeset/x-y_z", "'changeset/x-y_z'"},
		{"has space", "''"},    // invalid → no-op empty literal
		{"a;drop table", "''"}, // injection attempt neutralized
		{"quote'", "''"},
	}
	for _, c := range cases {
		if got := quoteRef(c.ref); got != c.want {
			t.Errorf("quoteRef(%q)=%q want %q", c.ref, got, c.want)
		}
	}
}

func TestKeysAndAddAll(t *testing.T) {
	if got := keys(map[string]bool{}); got != nil {
		t.Errorf("keys(empty) = %v, want nil", got)
	}
	set := map[string]bool{}
	addAll(set, []string{"a", "b"})
	addAll(set, []string{"b", "c"}) // dedup on the shared key
	got := keys(set)
	sort.Strings(got)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("keys=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("keys[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

// --- diff.go pure helpers ---------------------------------------------------

func TestCellString(t *testing.T) {
	ts := time.Date(2026, 7, 2, 10, 30, 0, 0, time.UTC)
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{[]byte("bytes"), "bytes"},
		{"str", "str"},
		{ts, "2026-07-02T10:30:00Z"},
		{42, "42"},
	}
	for _, c := range cases {
		if got := cellString(c.in); got != c.want {
			t.Errorf("cellString(%v)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestLabelFromTargets(t *testing.T) {
	targets := []store.RefTargetRow{
		{Type: "requirement", Key: "ATT-FR-012", ID: "r1"},
		{Type: "spec", Key: "ADDS", ID: "s1"},
		{Type: "spec", Key: "att/adds.md", ID: "s1"}, // duplicate id; first wins
	}
	label := labelFromTargets(targets)
	if got := label("requirement", "r1"); got != "requirement:ATT-FR-012" {
		t.Errorf("label req = %q", got)
	}
	if got := label("spec", "s1"); got != "spec:ADDS" {
		t.Errorf("first target should win: got %q, want spec:ADDS", got)
	}
	if got := label("entity", "unknown"); got != "entity:unknown" {
		t.Errorf("fallback = %q, want entity:unknown", got)
	}
}

func TestSpecRefsFromTargets(t *testing.T) {
	targets := []store.RefTargetRow{
		{Type: "spec", Key: "ADDS", ID: "s1"}, // prefix listed first → wins
		{Type: "spec", Key: "att/adds.md", ID: "s1"},
		{Type: "requirement", Key: "ATT-FR-1", ID: "r1"},
	}
	m := specRefsFromTargets(targets)
	if m["s1"] != "ADDS" {
		t.Errorf("spec ref = %q, want ADDS (prefix wins)", m["s1"])
	}
	if _, ok := m["r1"]; ok {
		t.Error("non-spec target should not appear in spec ref map")
	}
}

func TestDocRefFor(t *testing.T) {
	specRef := map[string]string{"s1": "ADDS"}
	cases := []struct {
		name        string
		subjectType string
		subjectID   string
		m           map[string]string
		want        string
	}{
		{"spec", "spec", "s1", nil, "spec:ADDS"},
		{"spec unknown", "spec", "sX", nil, ""},
		{"requirement via to_spec_id", "requirement", "r1", map[string]string{"to_spec_id": "s1"}, "spec:ADDS"},
		{"requirement via from_spec_id", "requirement", "r1", map[string]string{"from_spec_id": "s1"}, "spec:ADDS"},
		{"requirement unknown spec", "requirement", "r1", map[string]string{"to_spec_id": "sX"}, ""},
		{"user_story", "user_story", "u1", map[string]string{"to_spec_id": "s1"}, "spec:ADDS"},
		{"entity via to_name", "entity", "e1", map[string]string{"to_name": "Student"}, "entity:Student"},
		{"entity via from_name", "entity", "e1", map[string]string{"from_name": "Teacher"}, "entity:Teacher"},
		{"entity no name", "entity", "e1", map[string]string{}, ""},
		{"deliverable default", "deliverable", "d1", nil, ""},
	}
	for _, c := range cases {
		if got := docRefFor(c.subjectType, c.subjectID, c.m, specRef); got != c.want {
			t.Errorf("%s: docRefFor=%q want %q", c.name, got, c.want)
		}
	}
}

// --- impact.go pure helper --------------------------------------------------

func TestSortLinks(t *testing.T) {
	ls := []ImpactLink{
		{Endpoint: "spec:B", Via: "ref"},
		{Endpoint: "spec:A", Via: "refines"},
		{Endpoint: "spec:A", Via: "depends_on"},
	}
	sortLinks(ls)
	want := []ImpactLink{
		{Endpoint: "spec:A", Via: "depends_on"},
		{Endpoint: "spec:A", Via: "refines"},
		{Endpoint: "spec:B", Via: "ref"},
	}
	for i := range want {
		if ls[i] != want[i] {
			t.Errorf("sortLinks[%d]=%v want %v", i, ls[i], want[i])
		}
	}
}

// --- edge.go ResolveRef (pure over an in-memory resolver) -------------------

func TestResolveRef(t *testing.T) {
	resolver := refs.NewResolver([]refs.Target{
		{Type: "requirement", Key: "ATT-FR-012", ID: "r1"},
		{Type: "spec", Key: "ADDS", ID: "s1"},
	})
	cases := []struct {
		name   string
		arg    string
		wantID string
		wantOK bool
	}{
		{"bare key defaults to REQ", "ATT-FR-012", "r1", true},
		{"typed spec ref", "SPEC:ADDS", "s1", true},
		{"wrapped in brackets", "[[SPEC:ADDS]]", "s1", true},
		{"whitespace trimmed", "  SPEC:ADDS  ", "s1", true},
		{"unknown key", "REQ:NOPE", "", false},
		{"unknown tag", "BOGUS:x", "", false},
		{"empty arg", "", "", false},
	}
	for _, c := range cases {
		tg, ok := ResolveRef(resolver, c.arg)
		if ok != c.wantOK {
			t.Errorf("%s: ResolveRef(%q) ok=%v want %v", c.name, c.arg, ok, c.wantOK)
			continue
		}
		if ok && tg.ID != c.wantID {
			t.Errorf("%s: ResolveRef(%q).ID=%q want %q", c.name, c.arg, tg.ID, c.wantID)
		}
	}
}

// --- refs.go: ScanRefs / IngestRefs / DanglingError -------------------------

func TestScanAndIngestRefs(t *testing.T) {
	resolver := refs.NewResolver([]refs.Target{
		{Type: "requirement", Key: "ATT-FR-012", ID: "r1"},
	})
	// A resolved reference from a different owner yields one target, no danglers.
	scanned := ScanRefs(resolver, "requirement", "owner-id", "see [[REQ:ATT-FR-012]]")
	if len(scanned.Targets) != 1 || scanned.Targets[0].ID != "r1" {
		t.Fatalf("expected one resolved target r1, got %+v", scanned.Targets)
	}
	if len(scanned.Dangling) != 0 {
		t.Fatalf("expected no danglers, got %+v", scanned.Dangling)
	}

	// A dangling reference is reported, not resolved.
	dang := ScanRefs(resolver, "requirement", "owner-id", "see [[REQ:GHOST-FR-1]]")
	if len(dang.Targets) != 0 || len(dang.Dangling) != 1 {
		t.Fatalf("expected 0 targets, 1 dangler, got %+v", dang)
	}

	// IngestRefs returns rewritten fields in order plus the resolved refs.
	rewritten, res := IngestRefs(resolver, "requirement", "owner-id", "see [[REQ:ATT-FR-012]]", "no refs here")
	if len(rewritten) != 2 {
		t.Fatalf("expected 2 rewritten fields, got %d", len(rewritten))
	}
	if len(res.Targets) != 1 {
		t.Fatalf("expected 1 ingested target, got %+v", res.Targets)
	}
}

func TestDanglingError(t *testing.T) {
	if err := DanglingError(nil); err != nil {
		t.Errorf("no danglers should be nil error, got %v", err)
	}
	toks := refs.Scan("[[REQ:GHOST-FR-1]] and [[SPEC:NOPE]]")
	if len(toks) != 2 {
		t.Fatalf("scan setup: got %d tokens", len(toks))
	}
	err := DanglingError(toks)
	if err == nil {
		t.Fatal("expected an error for danglers")
	}
	var ce *CodedError
	if !errors.As(err, &ce) || ce.Code != ExitDangling {
		t.Errorf("expected a coded dangling error (exit %d), got %v", ExitDangling, err)
	}
}

// --- validate.go: ValidateEnum / ValidateRequired ---------------------------

func TestValidateEnum(t *testing.T) {
	allowed := []string{"active", "draft"}
	if err := ValidateEnum("status", "", allowed); err != nil {
		t.Errorf("empty value should pass: %v", err)
	}
	if err := ValidateEnum("status", "active", allowed); err != nil {
		t.Errorf("known value should pass: %v", err)
	}
	err := ValidateEnum("status", "bogus", allowed)
	if err == nil {
		t.Fatal("unknown value should fail")
	}
	var ce *CodedError
	if !errors.As(err, &ce) || ce.Code != ExitValidation {
		t.Errorf("expected exit %d validation error, got %v", ExitValidation, err)
	}
}

func TestValidateRequired(t *testing.T) {
	if err := ValidateRequired("name", "x"); err != nil {
		t.Errorf("non-empty should pass: %v", err)
	}
	err := ValidateRequired("name", "")
	if err == nil {
		t.Fatal("empty should fail")
	}
	var ce *CodedError
	if !errors.As(err, &ce) || ce.Code != ExitValidation {
		t.Errorf("expected exit %d, got %v", ExitValidation, err)
	}
}

// --- errors.go --------------------------------------------------------------

func TestCodedErrors(t *testing.T) {
	if coded(ExitGeneric, "x", nil) != nil {
		t.Error("coded(nil) should be nil")
	}

	nf := NotFound("branch", "feature-x")
	var ce *CodedError
	if !errors.As(nf, &ce) {
		t.Fatalf("NotFound should be a *CodedError, got %T", nf)
	}
	if ce.Code != ExitNotFound || ce.Category != "not_found" {
		t.Errorf("NotFound code/category = %d/%q", ce.Code, ce.Category)
	}
	if ce.Error() != `no branch "feature-x"` {
		t.Errorf("NotFound message = %q", ce.Error())
	}

	base := errors.New("root cause")
	wrapped := NotFoundErr(base)
	if !errors.Is(wrapped, base) {
		t.Error("NotFoundErr should unwrap to its cause")
	}
	var ce2 *CodedError
	if !errors.As(wrapped, &ce2) || ce2.Unwrap() != base {
		t.Error("Unwrap should return the wrapped error")
	}

	vf := ValidationFailed(errors.New("bad"))
	var ce3 *CodedError
	if !errors.As(vf, &ce3) || ce3.Code != ExitValidation {
		t.Errorf("ValidationFailed code = %v", vf)
	}
}
