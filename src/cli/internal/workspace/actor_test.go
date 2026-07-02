package workspace

import "testing"

// ResolveActor's git-config and identity-file fallbacks depend on the environment, so these
// cover the deterministic, short-circuiting precedence: an explicit --actor override and
// $CUSP_ACTOR both win outright over everything below them.

func TestResolveActor_HandleOverrideWins(t *testing.T) {
	t.Setenv("CUSP_ACTOR", "env-should-be-ignored")
	a := ResolveActor("bob")
	if a.Handle != "bob" || a.Name != "bob" {
		t.Fatalf("override should win: got handle=%q name=%q", a.Handle, a.Name)
	}
	if a.Email == "" {
		t.Fatalf("email should be non-empty")
	}
}

func TestResolveActor_CuspActorEnv(t *testing.T) {
	t.Setenv("CUSP_ACTOR", "carol")
	a := ResolveActor("")
	if a.Handle != "carol" {
		t.Fatalf("expected $CUSP_ACTOR to resolve, got %q", a.Handle)
	}
}

func TestIdentity_RoundTripAndMissing(t *testing.T) {
	dir := t.TempDir()

	// Missing file → zero Identity, no error.
	if id, err := LoadIdentity(dir); err != nil || (id != Identity{}) {
		t.Fatalf("missing identity: id=%+v err=%v", id, err)
	}

	want := Identity{Handle: "emalkoc", Name: "Ender Malkoc", Email: "ender@cusp.dev"}
	if err := SaveIdentity(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, want)
	}
}
