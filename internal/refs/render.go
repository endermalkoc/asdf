package refs

import "strings"

// RenderInline rewrites every resolvable `[[TYPE:key]]` token in text into a relative
// Markdown link and returns the rewritten text plus the tokens that did NOT resolve
// (unknown tag / TERM / dangling), which are left verbatim so the caller can report
// them. ownerDocPath is the slash path of the document containing text (used to make
// links relative). A target with no generated page renders as plain label text.
func RenderInline(text, ownerDocPath string, r *Resolver) (string, []Token) {
	toks := Scan(text)
	if len(toks) == 0 {
		return text, nil
	}
	var b strings.Builder
	var dangling []Token
	last := 0
	for _, t := range toks {
		b.WriteString(text[last:t.Start])
		last = t.End
		tg, ok := r.Resolve(t)
		if !ok {
			b.WriteString(t.Raw) // leave the token verbatim
			dangling = append(dangling, t)
			continue
		}
		b.WriteString(renderLink(t.Label(), ownerDocPath, tg))
	}
	b.WriteString(text[last:])
	return b.String(), dangling
}

// renderLink builds `[label](relpath#anchor)`, or just `label` when the target has no
// generated page (domain/milestone). A same-file target links in-page (`#anchor`).
func renderLink(label, ownerDocPath string, tg Target) string {
	if tg.DocPath == "" {
		return label
	}
	rel := ""
	if tg.DocPath != ownerDocPath {
		rel = relSlash(dirSlash(ownerDocPath), tg.DocPath)
	}
	if tg.Anchor != "" {
		rel += "#" + tg.Anchor
	}
	return "[" + label + "](" + rel + ")"
}

// dirSlash returns the directory portion of a slash path ("a/b/c.md" → "a/b",
// "c.md" → "").
func dirSlash(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return ""
}

// relSlash computes a slash-relative path from baseDir to the target file (both are
// repo-relative slash paths). It is the slash-only analog of filepath.Rel, so output
// never depends on the host OS separator.
func relSlash(baseDir, target string) string {
	base := splitNonEmpty(strings.Trim(baseDir, "/"))
	tgt := splitNonEmpty(strings.Trim(target, "/"))
	i := 0
	for i < len(base) && i < len(tgt) && base[i] == tgt[i] {
		i++
	}
	var rel []string
	for j := i; j < len(base); j++ {
		rel = append(rel, "..")
	}
	rel = append(rel, tgt[i:]...)
	return strings.Join(rel, "/")
}

func splitNonEmpty(p string) []string {
	if p == "" || p == "." {
		return nil
	}
	return strings.Split(p, "/")
}
