package tutor

import (
	"path"
	"regexp"
	"strings"

	"github.com/endermalkoc/cusp/internal/importer"
	"github.com/endermalkoc/cusp/internal/refs"
)

// mdLinkRe matches a standard Markdown link `[label](url)`.
var mdLinkRe = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

// convertRefs canonicalizes the corpus's prose into Cusp's inline-link grammar and
// derives the staging EntityRef set from it. It runs in two phases:
//
//  1. Rewrite every body-text field to canonical `[[TYPE:key]]` tokens. The corpus's
//     relative `[label](./other.md)` cross-spec/entity links are resolved against the
//     file tree (structural — needs the source layout, so it stays here), and bare inline
//     FR ids are canonicalized by the SHARED refs.Canonicalize — the exact step the CLI
//     write path runs, so a link is generated identically however a record is created.
//  2. Derive entity_refs from those tokens with the SHARED refs.ScanResolved (again, the
//     same scanner the CLI uses), attributing each ref to its owning entity.
//
// Unresolved corpus links are summed into one report finding.
func convertRefs(g *importer.Graph, rep *importer.Report) []importer.EntityRef {
	c := newRefConverter(g)
	resolver := graphResolver(g)

	// Phase 1 — rewrite text to canonical tokens (md-links → tokens, then bare FR → tokens).
	// The owner (type,key) lets Canonicalize skip a self-link — it matters for a
	// requirement's own statement; for spec/entity-owned prose it never self-matches.
	rewrite := func(s, dir, ownerType, ownerKey string) string {
		return refs.Canonicalize(c.convertLinks(s, dir), resolver, ownerType, ownerKey)
	}
	for i := range g.Specs {
		sp := &g.Specs[i]
		dir := path.Dir(sp.Path)
		for j := range sp.Sections {
			sp.Sections[j].Body = rewrite(sp.Sections[j].Body, dir, "spec", sp.Path)
		}
		for j := range sp.ReqGroups {
			sp.ReqGroups[j].Notes = rewrite(sp.ReqGroups[j].Notes, dir, "spec", sp.Path)
		}
	}
	for i := range g.Reqs {
		r := &g.Reqs[i]
		r.Statement = rewrite(r.Statement, c.dirByPrefix[r.SpecPrefix], "requirement", r.FRKey)
	}
	for i := range g.Stories {
		us := &g.Stories[i]
		dir := c.dirByPrefix[us.SpecPrefix]
		owner := c.pathByPrefix[us.SpecPrefix]
		us.Narrative = rewrite(us.Narrative, dir, "spec", owner)
		us.AsA = rewrite(us.AsA, dir, "spec", owner)
		us.IWant = rewrite(us.IWant, dir, "spec", owner)
		us.SoThat = rewrite(us.SoThat, dir, "spec", owner)
		us.WhyPriority = rewrite(us.WhyPriority, dir, "spec", owner)
		us.IndependentTest = rewrite(us.IndependentTest, dir, "spec", owner)
	}
	for i := range g.Scenarios {
		sc := &g.Scenarios[i]
		dir := c.dirByPrefix[sc.SpecPrefix]
		owner := c.pathByPrefix[sc.SpecPrefix]
		sc.Given = rewrite(sc.Given, dir, "spec", owner)
		sc.When = rewrite(sc.When, dir, "spec", owner)
		sc.Then = rewrite(sc.Then, dir, "spec", owner)
	}
	for i := range g.Entities {
		e := &g.Entities[i]
		dir := path.Dir(e.DocPath)
		for j := range e.Sections {
			e.Sections[j].Body = rewrite(e.Sections[j].Body, dir, "entity", e.Name)
		}
	}

	// Phase 2 — derive entity_refs from the canonical tokens, attributed to their owner.
	var out []importer.EntityRef
	seen := map[string]bool{}
	addRef := func(ownerType, ownerKey, targetType, targetKey string) {
		if ownerKey == "" || targetKey == "" || (ownerType == targetType && ownerKey == targetKey) {
			return
		}
		k := ownerType + "\x1f" + ownerKey + "\x1f" + targetType + "\x1f" + targetKey
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, importer.EntityRef{
			OwnerType: ownerType, OwnerKey: ownerKey,
			TargetType: targetType, TargetKey: targetKey,
		})
	}
	emit := func(ownerType, ownerKey string, fields ...string) {
		targets, _ := refs.ScanResolved(resolver, ownerType, ownerKey, fields...)
		for _, tg := range targets {
			addRef(ownerType, ownerKey, tg.Type, tg.ID)
		}
	}

	// Spec-owned text: sections + group notes + the spec's stories and scenarios.
	specFields := map[string][]string{}
	for i := range g.Specs {
		sp := &g.Specs[i]
		for _, s := range sp.Sections {
			specFields[sp.Path] = append(specFields[sp.Path], s.Body)
		}
		for _, gr := range sp.ReqGroups {
			specFields[sp.Path] = append(specFields[sp.Path], gr.Notes)
		}
	}
	for _, us := range g.Stories {
		if p, ok := c.pathByPrefix[us.SpecPrefix]; ok {
			specFields[p] = append(specFields[p], us.Narrative, us.AsA, us.IWant, us.SoThat, us.WhyPriority, us.IndependentTest)
		}
	}
	for _, sc := range g.Scenarios {
		if p, ok := c.pathByPrefix[sc.SpecPrefix]; ok {
			specFields[p] = append(specFields[p], sc.Given, sc.When, sc.Then)
		}
	}
	for i := range g.Specs {
		sp := &g.Specs[i]
		emit("spec", sp.Path, specFields[sp.Path]...)
		// Key Entities → spec→entity (a structured list, not a prose token).
		for _, name := range sp.KeyEntities {
			if c.entityNames[name] {
				addRef("spec", sp.Path, "entity", name)
			}
		}
	}
	for i := range g.Reqs {
		r := &g.Reqs[i]
		emit("requirement", r.FRKey, r.Statement)
	}
	for i := range g.Entities {
		e := &g.Entities[i]
		fields := make([]string, 0, len(e.Sections))
		for _, s := range e.Sections {
			fields = append(fields, s.Body)
		}
		emit("entity", e.Name, fields...)
	}

	if c.unresolved > 0 {
		rep.Add(importer.SevInfo, "unresolved-ref",
			itoa(c.unresolved)+" inline links/tokens did not resolve to an imported entity (cross-spec to a non-imported doc, or a typo)", "")
	}
	return out
}

// graphResolver builds a refs.Resolver over the staging graph — the (type,key)→Target
// index that refs.Canonicalize and refs.ScanResolved resolve against, mirroring the
// store-backed resolver the CLI uses. Each Target's ID is the entity's business key (the
// value apply.go resolves to a row id), so a scanned ref's TargetKey is what apply maps.
// Specs are indexed by both prefix and path; both point at the path (specByPath's key).
func graphResolver(g *importer.Graph) *refs.Resolver {
	var t []refs.Target
	for _, d := range g.Domains {
		t = append(t, refs.Target{Type: refs.TypeDomain, Key: d.Slug, ID: d.Slug})
	}
	for _, m := range g.Milestones {
		t = append(t, refs.Target{Type: refs.TypeMilestone, Key: m.Slug, ID: m.Slug})
	}
	for i := range g.Specs {
		sp := &g.Specs[i]
		t = append(t, refs.Target{Type: refs.TypeSpec, Key: sp.Path, ID: sp.Path})
		if sp.Prefix != "" {
			t = append(t, refs.Target{Type: refs.TypeSpec, Key: sp.Prefix, ID: sp.Path})
		}
	}
	for _, r := range g.Reqs {
		t = append(t, refs.Target{Type: refs.TypeRequirement, Key: r.FRKey, ID: r.FRKey})
	}
	for _, e := range g.Entities {
		t = append(t, refs.Target{Type: refs.TypeEntity, Key: e.Name, ID: e.Name})
	}
	return refs.NewResolver(t)
}

// refConverter holds the file-tree indexes the corpus md-link conversion resolves
// against (the structural, importer-only step in phase 1).
type refConverter struct {
	specByPath   map[string]*importer.Spec
	pathByPrefix map[string]string // spec prefix → source path (story/scenario owner)
	dirByPrefix  map[string]string // spec prefix → source dir (relative-link base)
	entityNames  map[string]bool
	entityByPath map[string]string // entity doc path → entity name
	frKeyByLower map[string]string // lowercased fr_key → fr_key (anchor resolution)

	unresolved int
}

func newRefConverter(g *importer.Graph) *refConverter {
	c := &refConverter{
		specByPath:   map[string]*importer.Spec{},
		pathByPrefix: map[string]string{},
		dirByPrefix:  map[string]string{},
		entityNames:  map[string]bool{},
		entityByPath: map[string]string{},
		frKeyByLower: map[string]string{},
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
		c.frKeyByLower[strings.ToLower(r.FRKey)] = r.FRKey
	}
	return c
}

// convertLinks rewrites the corpus's relative `[label](./other.md)` Markdown links into
// canonical `[[SPEC:..]]` / `[[REQ:..]]` / `[[ENTITY:..]]` tokens, resolving the relative
// path against dir (the owner document's directory). Non-relative or non-`.md` links are
// left untouched; a `.md` link that resolves to no imported entity is left as-is and
// counted as unresolved.
func (c *refConverter) convertLinks(text, dir string) string {
	if text == "" {
		return text
	}
	return mdLinkRe.ReplaceAllStringFunc(text, func(m string) string {
		sub := mdLinkRe.FindStringSubmatch(m)
		tok, ok := c.linkToToken(sub[1], strings.TrimSpace(sub[2]), dir)
		if !ok {
			if c.isRelMD(sub[2]) {
				c.unresolved++
			}
			return m
		}
		return tok
	})
}

// linkToToken resolves a Markdown link to a canonical token. ok is false for a
// non-relative-`.md` link (left untouched) and for a `.md` link that does not resolve to
// an imported spec/entity.
func (c *refConverter) linkToToken(label, url, dir string) (token string, ok bool) {
	if !c.isRelMD(url) {
		return "", false
	}
	rel, anchor := splitAnchor(url)
	target := path.Clean(path.Join(dir, rel))
	if name, isEnt := c.entityByPath[target]; isEnt {
		return "[[ENTITY:" + name + "|" + label + "]]", true
	}
	sp, isSpec := c.specByPath[target]
	if !isSpec {
		return "", false
	}
	if anchor != "" {
		if fk := c.frKeyByLower[strings.ToLower(anchor)]; fk != "" {
			return "[[REQ:" + fk + "|" + label + "]]", true
		}
	}
	key := sp.Prefix
	if key == "" {
		key = sp.Path
	}
	return "[[SPEC:" + key + "|" + label + "]]", true
}

// isRelMD reports whether url is a relative link to a `.md` file (the only links
// convertLinks rewrites).
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
