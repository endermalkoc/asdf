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
//	handleOverride (--actor) > $CUSP_ACTOR > `git config user.name` > $USER > "unknown".
//
// Email comes from `git config user.email`, else "<name>@cusp.local".
func ResolveActor(handleOverride string) Actor {
	name := firstNonEmpty(
		handleOverride,
		os.Getenv("CUSP_ACTOR"),
		gitConfig("user.name"),
		os.Getenv("USER"),
		"unknown",
	)
	email := firstNonEmpty(gitConfig("user.email"), name+"@cusp.local")
	return Actor{Handle: name, Name: name, Email: email}
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
