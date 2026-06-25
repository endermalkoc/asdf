// Package refs is the cross-reference layer: the `[[TYPE:key]]` inline-link grammar,
// a (type,key)→target resolver, and a Markdown renderer. It is pure — no database,
// no I/O — so it is shared by `generate` (render), the write path (validate), and the
// importer (scan), none of which it depends on. Callers load the resolver's targets
// from the store; the importer resolves against its own staging graph.
//
// Token form: `[[TYPE:key]]` with an optional display, `[[REQ:ATT-FR-012|the rule]]`.
// Types map to existing business keys — DOMAIN(abbreviation), SPEC(prefix or path),
// REQ(fr_key), ENTITY(name), MILESTONE(abbreviation), TERM(glossary slug or alias).
package refs

import (
	"regexp"
	"strings"
)

// Canonical entity types a token can target (the `owner_type`/`target_type` values).
const (
	TypeDomain      = "domain"
	TypeSpec        = "spec"
	TypeRequirement = "requirement"
	TypeEntity      = "entity"
	TypeMilestone   = "milestone"
	TypeTerm        = "glossary_term" // the [[TERM:slug]] tag resolves to a glossary_term
)

// tagToType maps an uppercase token tag to its canonical entity type.
var tagToType = map[string]string{
	"DOMAIN":    TypeDomain,
	"SPEC":      TypeSpec,
	"REQ":       TypeRequirement,
	"ENTITY":    TypeEntity,
	"MILESTONE": TypeMilestone,
	"TERM":      TypeTerm,
}

// tokenRe matches `[[TAG:key]]` with an optional `|display`. The key excludes `]`/`|`;
// the display excludes `]`.
var tokenRe = regexp.MustCompile(`\[\[([A-Za-z_]+):([^\]|]+?)(?:\|([^\]]+))?\]\]`)

// Token is one parsed `[[TYPE:key|display]]` occurrence in a text field.
type Token struct {
	Tag     string // raw uppercase tag, e.g. "REQ"
	Type    string // canonical entity type (TypeRequirement, …); "" if the tag is unknown
	Key     string // business key, e.g. "ATT-FR-012"
	Display string // optional display text; "" if absent
	Raw     string // the full matched token, e.g. "[[REQ:ATT-FR-012]]"
	Start   int    // byte offset of Raw in the source text
	End     int    // byte offset just past Raw
}

// Known reports whether the token's tag is a recognized entity type (DOMAIN/SPEC/
// REQ/ENTITY/MILESTONE/TERM); an unknown tag is not.
func (t Token) Known() bool { return t.Type != "" }

// Label is the text shown for a rendered link: the explicit display, else the key.
func (t Token) Label() string {
	if t.Display != "" {
		return t.Display
	}
	return t.Key
}

// Scan returns every `[[TYPE:key]]` token in text, in source order.
func Scan(text string) []Token {
	if !strings.Contains(text, "[[") {
		return nil
	}
	var out []Token
	for _, m := range tokenRe.FindAllStringSubmatchIndex(text, -1) {
		tag := strings.ToUpper(text[m[2]:m[3]])
		display := ""
		if m[6] >= 0 {
			display = strings.TrimSpace(text[m[6]:m[7]])
		}
		out = append(out, Token{
			Tag:     tag,
			Type:    tagToType[tag],
			Key:     strings.TrimSpace(text[m[4]:m[5]]),
			Display: display,
			Raw:     text[m[0]:m[1]],
			Start:   m[0],
			End:     m[1],
		})
	}
	return out
}
