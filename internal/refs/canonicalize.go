package refs

import "regexp"

// refMentionRe matches, in priority order, an existing `[[token]]`, a `[md](link)`, or a
// bare inline requirement id (the default `PREFIX-FR-NNN` key convention, optional
// sub-letter). Leftmost-match means an id already inside a token or a link is consumed
// by that earlier alternative and left untouched. The FR-key shape is a sensible default
// for the requirement-key convention (configurable policy, not core law); the resolver
// gate below means only ids that name a real requirement ever become links.
var refMentionRe = regexp.MustCompile(`\[\[[^\]]*\]\]|\[[^\]]*\]\([^)]*\)|\b[A-Z]{2,6}-FR-\d{2,3}[a-z]?\b`)

// Canonicalize rewrites bare inline requirement mentions (e.g. `**ATT-FR-012**`, or a
// plain "per ADDS-FR-034" in prose) into `[[REQ:key]]` tokens, so a later render links
// them. Only ids the resolver knows become tokens; an unknown id — and any id already
// inside a `[[token]]` or a `[md](link)` — is left as-is. Hand-written `[[TYPE:key]]`
// tokens pass through unchanged. A mention of the owner itself (ownerType/ownerID, e.g.
// a requirement's statement naming its own key) is left as plain text — a self-link is
// noise — symmetric with ScanResolved dropping the self-reference. Pass ownerID "" (and/
// or ownerType "") when there is no owner to exclude.
//
// This is the single canonicalization step shared by the write path (CLI) and the
// importer, so a link is generated identically however a record is created. (Corpus-
// structural rewrites — resolving a relative `[x](./y.md)` against a file tree — are not
// here; they need the source layout and stay in the importer.)
func Canonicalize(text string, r *Resolver, ownerType, ownerID string) string {
	if text == "" || r == nil {
		return text
	}
	return refMentionRe.ReplaceAllStringFunc(text, func(m string) string {
		if m[0] == '[' { // an existing [[token]] or [md](link)
			return m
		}
		tg, ok := r.Resolve(Token{Type: TypeRequirement, Key: m})
		if !ok {
			return m // not a known requirement id — leave as text
		}
		if tg.Type == ownerType && tg.ID == ownerID {
			return m // self-reference — don't link a record to itself
		}
		return "[[REQ:" + tg.Key + "]]"
	})
}

// ScanResolved scans every field for `[[TYPE:key]]` tokens and resolves each against r.
// It returns the distinct resolved targets — deduplicated by (type, id), with any
// self-reference to (ownerType, ownerID) dropped — and the tokens that did not resolve
// (danglers, for the caller to block an interactive write or report on import). ownerID
// may be "" when the owner row does not exist yet (validating a create before its id is
// minted); real targets always carry a non-empty id, so nothing is mistaken for a self-
// reference in that case. Shared by the CLI write path and the importer.
func ScanResolved(r *Resolver, ownerType, ownerID string, fields ...string) (targets []Target, dangling []Token) {
	seen := map[string]bool{}
	for _, f := range fields {
		for _, t := range Scan(f) {
			tg, ok := r.Resolve(t)
			if !ok {
				dangling = append(dangling, t)
				continue
			}
			if tg.Type == ownerType && tg.ID == ownerID {
				continue // self-reference
			}
			k := tg.Type + "\x00" + tg.ID
			if seen[k] {
				continue
			}
			seen[k] = true
			targets = append(targets, tg)
		}
	}
	return targets, dangling
}
