package agentsetup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Marker-delimited managed blocks keep a Cusp section idempotent inside an agent instructions
// file (CLAUDE.md / AGENTS.md). The block is wrapped in HTML comments carrying a schema version
// and a content hash:
//
//	<!-- BEGIN CUSP INTEGRATION v:1 hash:ab12cd34 -->
//	…section body…
//	<!-- END CUSP INTEGRATION -->
//
// Re-running setup replaces the block in place only when the content changed, appends it if the
// file has no block yet, and creates the file (with a small scaffold) if it is absent — leaving
// any surrounding user content untouched.

const (
	markerVersion  = 1
	endMarker      = "<!-- END CUSP INTEGRATION -->"
	scaffoldHeader = "# Project instructions for AI agents\n\n"
)

// blockRe matches the whole managed block (begin marker … end marker), plus a trailing newline.
var blockRe = regexp.MustCompile(`(?s)<!-- BEGIN CUSP INTEGRATION.*?<!-- END CUSP INTEGRATION -->\n?`)

// InstallAction reports what InstallSection did to the target file.
type InstallAction string

const (
	ActionCreated   InstallAction = "created"
	ActionAppended  InstallAction = "appended"
	ActionUpdated   InstallAction = "updated"
	ActionUnchanged InstallAction = "unchanged"
)

func bodyHash(body string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(body)))
	return hex.EncodeToString(sum[:])[:8]
}

// renderBlock wraps a section body in the versioned, hashed markers.
func renderBlock(body string) string {
	body = strings.TrimRight(body, "\n")
	return fmt.Sprintf("<!-- BEGIN CUSP INTEGRATION v:%d hash:%s -->\n%s\n%s\n", markerVersion, bodyHash(body), body, endMarker)
}

// InstallSection idempotently upserts the managed Cusp block into the file at path, refusing to
// write through a symlink. Returns what it did (created / appended / updated / unchanged).
func InstallSection(path, body string) (InstallAction, error) {
	if err := guardNotSymlink(path); err != nil {
		return "", err
	}
	block := renderBlock(body)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if werr := os.WriteFile(path, []byte(scaffoldHeader+block), 0o644); werr != nil {
			return "", werr
		}
		return ActionCreated, nil
	}
	if err != nil {
		return "", err
	}
	content := string(data)
	if loc := blockRe.FindStringIndex(content); loc != nil {
		if strings.TrimRight(content[loc[0]:loc[1]], "\n") == strings.TrimRight(block, "\n") {
			return ActionUnchanged, nil
		}
		updated := content[:loc[0]] + block + content[loc[1]:]
		if werr := os.WriteFile(path, []byte(updated), 0o644); werr != nil {
			return "", werr
		}
		return ActionUpdated, nil
	}
	// No block yet — append after existing content with one blank line before it.
	updated := strings.TrimRight(content, "\n") + "\n\n" + block
	if werr := os.WriteFile(path, []byte(updated), 0o644); werr != nil {
		return "", werr
	}
	return ActionAppended, nil
}

// RemoveSection deletes the managed block from path; returns whether it removed anything.
func RemoveSection(path string) (bool, error) {
	if err := guardNotSymlink(path); err != nil {
		return false, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	content := string(data)
	loc := blockRe.FindStringIndex(content)
	if loc == nil {
		return false, nil
	}
	head := strings.TrimRight(content[:loc[0]], "\n")
	tail := content[loc[1]:]
	updated := head
	if head != "" && tail != "" {
		updated += "\n"
	}
	updated += tail
	// Normalize to a single trailing newline (or empty), so removing our block doesn't strip
	// the file's final newline.
	if updated = strings.TrimRight(updated, "\n"); updated != "" {
		updated += "\n"
	}
	if werr := os.WriteFile(path, []byte(updated), 0o644); werr != nil {
		return false, werr
	}
	return true, nil
}

// SectionStatus reports whether the managed block is present in path and, if so, whether it is
// current (its embedded hash matches the given body). For `cusp setup --check`.
func SectionStatus(path, body string) (present, current bool, err error) {
	data, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return false, false, nil
	}
	if e != nil {
		return false, false, e
	}
	loc := blockRe.FindStringIndex(string(data))
	if loc == nil {
		return false, false, nil
	}
	existing := string(data)[loc[0]:loc[1]]
	return true, strings.Contains(existing, "hash:"+bodyHash(body)+" "), nil
}

func guardNotSymlink(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symlink; refusing to edit through it", path)
	}
	return nil
}
