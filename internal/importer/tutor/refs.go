package tutor

import (
	"path"
	"regexp"
	"strings"

	"github.com/endermalkoc/asdf/internal/importer"
	"github.com/endermalkoc/asdf/internal/refs"
)

// mdLinkRe matches a standard Markdown link `[label](url)`.
var mdLinkRe = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

// convertRefs derives the staging EntityRef set and, as a side effect, rewrites the
// corpus's relative `[label](./other.md)` cross-spec links (and `[..](../entities/x.md)`
// entity links) into canonical `[[SPEC:..]]` / `[[REQ:..]]` / `[[ENTITY:..]]` tokens in
// the stored text — so a regenerate renders them as links. It also records inline FR
// mentions in requirement statements and each spec's Key Entities. Unresolved
// links/tokens are summed into one report finding.
func convertRefs(g *importer.Graph, rep *importer.Report) []importer.EntityRef {
	c := newRefConverter(g)

	// Key Entities: spec → entity references.
	for _, sp := range g.Specs {
		for _, name := range sp.KeyEntities {
			if c.entityNames[name] {
				c.add("spec", sp.Path, "entity", name)
			}
		}
	}

	// Spec-owned text: every section (keyed + bespoke) lives in sp.Sections now,
	// plus the FR-group notes.
	for i := range g.Specs {
		sp := &g.Specs[i]
		dir := path.Dir(sp.Path)
		conv := func(s string) string { return c.convert(s, "spec", sp.Path, dir) }
		for j := range sp.Sections {
			sp.Sections[j].Body = conv(sp.Sections[j].Body)
		}
		for j := range sp.ReqGroups {
			sp.ReqGroups[j].Note = conv(sp.ReqGroups[j].Note)
		}
	}

	// Requirement statements: convert links + record bare FR mentions (owner = the FR).
	for i := range g.Reqs {
		r := &g.Reqs[i]
		r.Statement = c.convert(r.Statement, "requirement", r.FRKey, c.dirByPrefix[r.SpecPrefix])
		c.bareFRMentions(r.Statement, r.FRKey)
	}

	// Story + scenario text → attributed to the owning spec.
	for i := range g.Stories {
		us := &g.Stories[i]
		ownerPath, dir := c.pathByPrefix[us.SpecPrefix], c.dirByPrefix[us.SpecPrefix]
		conv := func(s string) string { return c.convert(s, "spec", ownerPath, dir) }
		us.Narrative = conv(us.Narrative)
		us.AsA = conv(us.AsA)
		us.IWant = conv(us.IWant)
		us.SoThat = conv(us.SoThat)
		us.WhyPriority = conv(us.WhyPriority)
		us.IndependentTest = conv(us.IndependentTest)
	}
	for i := range g.Scenarios {
		sc := &g.Scenarios[i]
		ownerPath, dir := c.pathByPrefix[sc.SpecPrefix], c.dirByPrefix[sc.SpecPrefix]
		sc.Given = c.convert(sc.Given, "spec", ownerPath, dir)
		sc.When = c.convert(sc.When, "spec", ownerPath, dir)
		sc.Then = c.convert(sc.Then, "spec", ownerPath, dir)
	}

	// Entity-owned text: every section (keyed + bespoke) lives in e.Sections now.
	for i := range g.Entities {
		e := &g.Entities[i]
		dir := path.Dir(e.DocPath)
		conv := func(s string) string { return c.convert(s, "entity", e.Name, dir) }
		for j := range e.Sections {
			e.Sections[j].Body = conv(e.Sections[j].Body)
		}
	}

	if c.unresolved > 0 {
		rep.Add(importer.SevInfo, "unresolved-ref",
			itoa(c.unresolved)+" inline links/tokens did not resolve to an imported entity (cross-spec to a non-imported doc, or a typo)", "")
	}
	return c.out
}

// refConverter holds the business-key indexes a conversion pass resolves against.
type refConverter struct {
	specByPath   map[string]*importer.Spec
	pathByPrefix map[string]string
	dirByPrefix  map[string]string
	entityNames  map[string]bool
	entityByPath map[string]string // entity doc path → entity name
	frKeys       map[string]bool
	frKeyByLower map[string]string
	domainAbbr   map[string]bool
	milestoneAbb map[string]bool

	out        []importer.EntityRef
	seen       map[string]bool
	unresolved int
}

func newRefConverter(g *importer.Graph) *refConverter {
	c := &refConverter{
		specByPath:   map[string]*importer.Spec{},
		pathByPrefix: map[string]string{},
		dirByPrefix:  map[string]string{},
		entityNames:  map[string]bool{},
		entityByPath: map[string]string{},
		frKeys:       map[string]bool{},
		frKeyByLower: map[string]string{},
		domainAbbr:   map[string]bool{},
		milestoneAbb: map[string]bool{},
		seen:         map[string]bool{},
	}
	for i := range g.Specs {
		sp := &g.Specs[i]
		c.specByPath[sp.Path] = sp
		if sp.Prefix != "" {
			c.pathByPrefix[sp.Prefix] = sp.Path
			c.dirByPrefix[sp.Prefix] = path.Dir(sp.Path)
		}
	}
	for _, e := range g.Entities {
		c.entityNames[e.Name] = true
		if e.DocPath != "" {
			c.entityByPath[e.DocPath] = e.Name
		}
	}
	for _, r := range g.Reqs {
		c.frKeys[r.FRKey] = true
		c.frKeyByLower[strings.ToLower(r.FRKey)] = r.FRKey
	}
	for _, d := range g.Domains {
		c.domainAbbr[d.Abbrev] = true
	}
	for _, m := range g.Milestones {
		c.milestoneAbb[m.Abbrev] = true
	}
	return c
}

// add records a de-duplicated, non-self reference.
func (c *refConverter) add(ownerType, ownerKey, targetType, targetKey string) {
	if ownerKey == "" || targetKey == "" {
		return
	}
	if ownerType == targetType && ownerKey == targetKey {
		return
	}
	k := ownerType + "\x1f" + ownerKey + "\x1f" + targetType + "\x1f" + targetKey
	if c.seen[k] {
		return
	}
	c.seen[k] = true
	c.out = append(c.out, importer.EntityRef{
		OwnerType: ownerType, OwnerKey: ownerKey,
		TargetType: targetType, TargetKey: targetKey, Kind: "references",
	})
}

// convert rewrites relative-`.md` Markdown links in text into canonical tokens
// (recording each reference), records references for any `[[TYPE:key]]` tokens
// present, and returns the (possibly rewritten) text. dir is the owner document's
// directory, for resolving relative links.
func (c *refConverter) convert(text, ownerType, ownerKey, dir string) string {
	if text == "" {
		return text
	}
	text = mdLinkRe.ReplaceAllStringFunc(text, func(m string) string {
		sub := mdLinkRe.FindStringSubmatch(m)
		tok, tType, tKey, ok := c.linkToToken(sub[1], strings.TrimSpace(sub[2]), dir)
		if !ok {
			if c.isRelMD(sub[2]) {
				c.unresolved++
			}
			return m
		}
		c.add(ownerType, ownerKey, tType, tKey)
		return tok
	})
	for _, t := range refs.Scan(text) {
		tType, tKey, ok := c.resolveToken(t)
		if !ok {
			if t.Known() {
				c.unresolved++
			}
			continue
		}
		c.add(ownerType, ownerKey, tType, tKey)
	}
	return text
}

// linkToToken resolves a Markdown link to a canonical token + reference target. ok is
// false for non-relative-`.md` links (left untouched) and for `.md` links that do not
// resolve to an imported spec/entity.
func (c *refConverter) linkToToken(label, url, dir string) (token, targetType, targetKey string, ok bool) {
	if !c.isRelMD(url) {
		return "", "", "", false
	}
	rel, anchor := splitAnchor(url)
	target := path.Clean(path.Join(dir, rel))
	if name, isEnt := c.entityByPath[target]; isEnt {
		return "[[ENTITY:" + name + "|" + label + "]]", "entity", name, true
	}
	sp, isSpec := c.specByPath[target]
	if !isSpec {
		return "", "", "", false
	}
	if anchor != "" {
		if fk := c.frKeyByLower[strings.ToLower(anchor)]; fk != "" {
			return "[[REQ:" + fk + "|" + label + "]]", "requirement", fk, true
		}
	}
	key := sp.Prefix
	if key == "" {
		key = sp.Path
	}
	return "[[SPEC:" + key + "|" + label + "]]", "spec", sp.Path, true
}

// resolveToken maps a `[[TYPE:key]]` token to its (targetType, targetKey) — the
// business key apply.resolve expects (spec → path, requirement → fr_key, …).
func (c *refConverter) resolveToken(t refs.Token) (string, string, bool) {
	switch t.Type {
	case refs.TypeDomain:
		if c.domainAbbr[t.Key] {
			return "domain", t.Key, true
		}
	case refs.TypeSpec:
		if p, ok := c.pathByPrefix[t.Key]; ok {
			return "spec", p, true
		}
		if _, ok := c.specByPath[t.Key]; ok {
			return "spec", t.Key, true
		}
	case refs.TypeRequirement:
		if c.frKeys[t.Key] {
			return "requirement", t.Key, true
		}
	case refs.TypeEntity:
		if c.entityNames[t.Key] {
			return "entity", t.Key, true
		}
	case refs.TypeMilestone:
		if c.milestoneAbb[t.Key] {
			return "milestone", t.Key, true
		}
	}
	return "", "", false
}

// bareFRMentions records requirement→requirement references for inline FR ids
// mentioned in a statement (e.g. "per ADDS-FR-034"), without rewriting the prose.
func (c *refConverter) bareFRMentions(statement, ownerFRKey string) {
	if statement == "" {
		return
	}
	for _, m := range frTokenRe.FindAllString(statement, -1) {
		target := m
		if p, n, suf, ok := splitFRKey(m); ok {
			target = frKey(p, n, suf)
		}
		if c.frKeys[target] {
			c.add("requirement", ownerFRKey, "requirement", target)
		}
	}
}

// isRelMD reports whether url is a relative link to a `.md` file (the only links the
// converter rewrites).
func (c *refConverter) isRelMD(url string) bool {
	u := strings.TrimSpace(url)
	low := strings.ToLower(u)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") ||
		strings.HasPrefix(low, "mailto:") || strings.HasPrefix(u, "#") || strings.HasPrefix(u, "/") {
		return false
	}
	base, _ := splitAnchor(u)
	return strings.HasSuffix(strings.ToLower(base), ".md")
}

// splitAnchor splits a url into its path and `#anchor` (anchor empty if none).
func splitAnchor(url string) (string, string) {
	if i := strings.IndexByte(url, '#'); i >= 0 {
		return url[:i], url[i+1:]
	}
	return url, ""
}
