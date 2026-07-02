package workspace

import (
	"os"
	"os/exec"
	"strings"
)

// Actor identifies who is making a change — for the Dolt commit author and the
// `rev_actor` table.
type Actor struct {
	Handle string // stable identity, e.g. "ender" or "agent:claude"
	Name   string // display name
	Email  string // for the Dolt commit author
}

// ResolveActor determines the current actor, in precedence order:
//
//	handleOverride (--actor) > $CUSP_ACTOR > persisted identity (.cusp/identity.json) >
//	`git config user.name` > $USER > "unknown".
//
// An explicit override (--actor / $CUSP_ACTOR) wins outright — handle, name, and email all
// follow it — so ad-hoc attribution ("--actor agent:claude") isn't blended with the persisted
// human identity. Otherwise the per-user identity file (set via `cusp config set user.*`) is
// consulted, then git config. Email comes from the identity, else `git config user.email`,
// else "<name>@cusp.local".
func ResolveActor(handleOverride string) Actor {
	if explicit := firstNonEmpty(handleOverride, os.Getenv("CUSP_ACTOR")); explicit != "" {
		email := firstNonEmpty(gitConfig("user.email"), explicit+"@cusp.local")
		return Actor{Handle: explicit, Name: explicit, Email: email}
	}
	id := currentIdentity()
	handle := firstNonEmpty(id.Handle, gitConfig("user.name"), os.Getenv("USER"), "unknown")
	name := firstNonEmpty(id.Name, handle)
	email := firstNonEmpty(id.Email, gitConfig("user.email"), name+"@cusp.local")
	return Actor{Handle: handle, Name: name, Email: email}
}

// CommitAuthorString formats the actor as a Dolt commit author: "Name <email>".
func (a Actor) CommitAuthorString() string {
	return a.Name + " <" + a.Email + ">"
}

func gitConfig(key string) string {
	out, err := exec.Command("git", "config", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
