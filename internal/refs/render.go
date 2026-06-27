package refs

import (
	"regexp"
	"strings"
)

// mdPathTokenRe matches a file-path token ending in `.md` inside a link label (e.g.
// `shared/contacts-notifications.md`), so it can be swapped for the target's title.
var mdPathTokenRe = regexp.MustCompile(`[^\s|\]]+\.md`)

// RenderInline rewrites every resolvable `[[TYPE:key]]` token in text into an Obsidian
// wikilink and returns the rewritten text plus the tokens that did NOT resolve
// (unknown tag / TERM / dangling), which are left verbatim so the caller can report
// them. ownerDocPath is the slash path of the document containing text (used to detect
// same-file links). A target with no generated page renders as plain label text.
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

// renderLink builds an Obsidian wikilink `[[target#^anchor|label]]` (vault-relative
// path, extension dropped; the anchor is a `^block` reference). A target with no
// generated page (domain/milestone) renders as plain label; a same-file target omits
// the path, giving `[[#^anchor|label]]`. HTML/relative links are reserved for the
// future HTML generator.
func renderLink(label, ownerDocPath string, tg Target) string {
	label = displayLabel(label, tg)
	if tg.DocPath == "" {
		return label
	}
	inner := ""
	if tg.DocPath != ownerDocPath {
		inner = strings.TrimSuffix(tg.DocPath, ".md") // vault-relative path, no extension
	}
	if tg.Anchor != "" {
		inner += "#^" + tg.Anchor // Obsidian block reference
	}
	if inner == "" {
		return label
	}
	return "[[" + inner + "|" + label + "]]"
}

// displayLabel cleans a link's display text: a source link whose text was a file path
// (`shared/contacts-notifications.md`) renders as the target's human title; any trailing
// descriptive suffix (`… — Email Field`) is preserved. A descriptive label with no path
// token (an fr_key, a phrase) is left untouched. With no title available, the path token at
// least loses its directory and `.md`.
func displayLabel(label string, tg Target) string {
	return mdPathTokenRe.ReplaceAllStringFunc(label, func(tok string) string {
		if tg.Label != "" {
			return tg.Label
		}
		base := tok[strings.LastIndexByte(tok, '/')+1:]
		return strings.TrimSuffix(base, ".md")
	})
}
