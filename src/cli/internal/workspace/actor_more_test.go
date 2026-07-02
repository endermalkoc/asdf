package workspace

import (
	"strings"
	"testing"
)

func TestCommitAuthorString(t *testing.T) {
	a := Actor{Handle: "ender", Name: "Ender Malkoc", Email: "ender@cusp.local"}
	if got, want := a.CommitAuthorString(), "Ender Malkoc <ender@cusp.local>"; got != want {
		t.Fatalf("CommitAuthorString = %q, want %q", got, want)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "x", "y"); got != "x" {
		t.Fatalf("firstNonEmpty skips blanks: got %q", got)
	}
	if got := firstNonEmpty("  spaced  "); got != "spaced" {
		t.Fatalf("firstNonEmpty trims: got %q", got)
	}
	if got := firstNonEmpty("", "   "); got != "" {
		t.Fatalf("firstNonEmpty all-blank: got %q", got)
	}
}

// With no --actor override and no $CUSP_ACTOR, ResolveActor falls through the identity /
// git-config / $USER chain. The exact values are environment-dependent, but the resolver
// must always return a fully-populated, non-blank Actor (worst case "unknown" + synthesized
// email). This drives the non-override branch of ResolveActor and currentIdentity().
func TestResolveActor_FallthroughIsPopulated(t *testing.T) {
	t.Setenv("CUSP_ACTOR", "")
	a := ResolveActor("")
	if strings.TrimSpace(a.Handle) == "" {
		t.Fatalf("handle must be non-empty, got %q", a.Handle)
	}
	if strings.TrimSpace(a.Name) == "" {
		t.Fatalf("name must be non-empty, got %q", a.Name)
	}
	if !strings.Contains(a.Email, "@") {
		t.Fatalf("email must look like an address, got %q", a.Email)
	}
}

// An explicit --actor override with no configured git email synthesizes "<handle>@cusp.local".
// Whether git provides an email is environment-dependent, so accept either the real git email
// or the synthesized fallback, but require it to be tied to a non-blank address.
func TestResolveActor_OverrideEmail(t *testing.T) {
	t.Setenv("CUSP_ACTOR", "")
	a := ResolveActor("agent:claude")
	if a.Handle != "agent:claude" || a.Name != "agent:claude" {
		t.Fatalf("override should set handle and name: %+v", a)
	}
	if !strings.Contains(a.Email, "@") {
		t.Fatalf("override email should be an address, got %q", a.Email)
	}
}
