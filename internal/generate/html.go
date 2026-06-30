package generate

import (
	"bytes"
	"html"
	"path"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

// The HTML renderer turns the Markdown pages into a self-contained static site: each
// Obsidian-Markdown page is rewritten into CommonMark goldmark understands (wikilinks →
// relative .html links, `^block` anchors → inline <a id> targets), converted to HTML, and
// wrapped in a page with an always-visible sidebar (the full project tree), breadcrumbs, and
// a linked stylesheet. So HTML is "Markdown for the browser" — same content, navigable in a
// static site instead of in Obsidian.
type htmlRenderer struct{ md goldmark.Markdown }

func newHTMLRenderer(m *Model) Renderer {
	return htmlRenderer{md: goldmark.New(
		goldmark.WithExtensions(extension.GFM),            // tables, etc.
		goldmark.WithRendererOptions(gmhtml.WithUnsafe()), // pass through our own <a id> anchors
	)}
}

func (r htmlRenderer) Render(m *Model) ([]File, error) {
	mdFiles, err := newMarkdownRenderer(m).Render(m)
	if err != nil {
		return nil, err
	}
	nav := m.Nav
	if nav == nil {
		nav = &Nav{}
	}
	planningBodies := planningHTMLBodies(m)
	out := make([]File, 0, len(mdFiles)+1)
	for _, f := range mdFiles {
		title := docTitle(f.Content)
		htmlPath := strings.TrimSuffix(f.Path, ".md") + ".html"
		rel := relPrefix(f.Path)
		var body string
		switch {
		case f.Path == "index.md":
			// The root index is a browsable tree (sitemap) of the whole project.
			body = "<h1>" + html.EscapeString(title) + "</h1>\n" +
				`<p class="lede">Browse the project — specifications by domain, and shared entities.</p>` +
				renderContentTree(nav, rel)
		case planningBodies[f.Path] != "":
			// Planning pages are rendered straight from the Model (pills, icons, tree) rather
			// than from Markdown, so the GFM conversion is skipped for them.
			body = planningBodies[f.Path]
		default:
			// Internal links get an xref class; priority badges and the frontmatter-sourced
			// metadata bar (under the H1) decorate the converted body.
			gfm := obsidianToGFM(f.Content, f.Path)
			var buf bytes.Buffer
			if err := r.md.Convert([]byte(gfm), &buf); err != nil {
				return nil, err
			}
			body = injectMetaBar(
				styleXrefLinks(decoratePriorities(buf.String())),
				renderMetaBar(parseFrontmatter(f.Content), nav, rel))
		}
		page := htmlPage(pageChrome{
			Title:       title,
			Body:        body,
			Rel:         rel,
			Sidebar:     renderSidebar(nav, htmlPath, rel),
			Breadcrumbs: renderBreadcrumbs(nav, f.Path, title, rel),
			PageIcon:    icon(pageKind(f.Path, f.Kind)),
		})
		out = append(out, File{Path: htmlPath, Content: page, Kind: f.Kind})
	}
	// The stylesheet is a static asset written once per build; on the incremental fast path
	// its content hash matches and it is skipped, so it costs nothing after the first write.
	out = append(out, File{Path: assetStylePath, Content: styleCSS, Kind: "asset"})
	return out, nil
}

// assetStylePath is where the default stylesheet is written, relative to the out dir.
const assetStylePath = "assets/style.css"

// renderSidebar builds the always-visible navigation tree, mirroring the document hierarchy:
// domains (and their nested sub-directories) → docs, then Entities, then Glossary. Directory
// groupings collapse via <details>; the current page's link gets the `active` class. cur is
// the current page's root-relative .html path; rel is the `../`-prefix back to the site root.
func renderSidebar(nav *Nav, cur, rel string) string {
	var b strings.Builder
	b.WriteString(`<nav class="nav-tree"><ul class="nav-root">`)
	for _, n := range nav.Root {
		b.WriteString(`<li>`)
		renderNavNode(&b, n, cur, rel)
		b.WriteString(`</li>`)
	}
	b.WriteString(`</ul></nav>`)
	return b.String()
}

// renderNavNode renders one tree node. A node with children becomes a collapsible <details>
// (its summary is the node's own link or, for a bare directory, a plain label); a childless
// node is a leaf link. Each entry carries a type icon. A group is rendered `open` only when
// the current page (cur) is somewhere in its subtree, so each page shows just its own branch
// expanded — navigating never re-expands the whole tree, and collapsed siblings stay tidy.
// (Manual expand/collapse is per-page, since the output is static HTML with no script.)
func renderNavNode(b *strings.Builder, n *NavNode, cur, rel string) {
	if len(n.Children) > 0 {
		open := ""
		if navContainsActive(n, cur) {
			open = " open"
		}
		b.WriteString(`<details class="nav-group"` + open + `><summary>`)
		if n.Href != "" {
			navLink(b, rel+n.Href, n.Label, n.Href == cur, "nav-self", n.Kind)
		} else {
			navDir(b, n.Label, n.Kind)
		}
		b.WriteString(`</summary><ul>`)
		for _, c := range n.Children {
			b.WriteString(`<li>`)
			renderNavNode(b, c, cur, rel)
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ul></details>`)
		return
	}
	if n.Href != "" {
		navLink(b, rel+n.Href, n.Label, n.Href == cur, "nav-leaf", n.Kind)
	} else {
		navDir(b, n.Label, n.Kind)
	}
}

// navContainsActive reports whether the current page (cur) is this node or anywhere beneath
// it — used to auto-expand only the branch leading to the current page.
func navContainsActive(n *NavNode, cur string) bool {
	if n.Href == cur {
		return true
	}
	for _, c := range n.Children {
		if navContainsActive(c, cur) {
			return true
		}
	}
	return false
}

// navLink writes one sidebar anchor with its type icon, adding the `active` class when it is
// the current page.
func navLink(b *strings.Builder, href, label string, active bool, class, kind string) {
	if active {
		class += " active"
	}
	b.WriteString(`<a class="` + class + `" href="` + href + `">` + icon(kind) +
		`<span class="nav-label">` + html.EscapeString(label) + `</span></a>`)
}

// navDir writes a non-linking directory grouping label with its folder icon.
func navDir(b *strings.Builder, label, kind string) {
	b.WriteString(`<span class="nav-dir">` + icon(kind) +
		`<span class="nav-label">` + html.EscapeString(label) + `</span></span>`)
}

// breadcrumb is one entry in the trail above the document.
type breadcrumb struct {
	Label   string
	Href    string // "" → not a link
	current bool   // the final, current page (rendered emphasized, never a link)
}

// renderBreadcrumbs builds the trail from the document's path, led by its top-level section.
// Spec pages live directly under a domain, so they lead with the Specifications section
// (Specifications / Domain / … / Title); entity pages already start with the "entities"
// segment, which renders as the Entities section; the glossary is its own section. Each
// directory segment links to its index page when it has one (domains do, sub-dirs do not).
func renderBreadcrumbs(nav *Nav, mdPath, title, rel string) string {
	// The root index is the Specifications landing itself.
	if mdPath == "index.md" {
		return renderCrumbs([]breadcrumb{{Label: "Specifications", current: true}})
	}

	segs := strings.Split(strings.TrimSuffix(mdPath, ".md"), "/")
	index := segs[len(segs)-1] == "index" // a directory landing page

	var cs []breadcrumb
	// Spec/domain pages aren't physically under a "specifications/" directory, so prepend the
	// section crumb; entity/planning/glossary pages carry their section in the path already.
	if segs[0] != "entities" && segs[0] != "planning" && mdPath != "glossary.md" {
		cs = append(cs, breadcrumb{Label: "Specifications", Href: rel + "index.html"})
	}
	for i, seg := range segs[:len(segs)-1] {
		dirPath := strings.Join(segs[:i+1], "/")
		c := breadcrumb{Label: nav.SegLabel(dirPath, seg)}
		if nav.HasIndex(dirPath) {
			c.Href = rel + dirPath + "/index.html"
		}
		cs = append(cs, c)
	}
	if index {
		cs[len(cs)-1].Href, cs[len(cs)-1].current = "", true // the dir's own index page
	} else {
		cs = append(cs, breadcrumb{Label: title, current: true})
	}
	return renderCrumbs(cs)
}

// renderCrumbs renders the breadcrumb list; non-current crumbs with an Href become links.
func renderCrumbs(cs []breadcrumb) string {
	var b strings.Builder
	b.WriteString(`<nav class="breadcrumbs">`)
	for i, c := range cs {
		if i > 0 {
			b.WriteString(`<span class="sep">/</span>`)
		}
		if c.current || c.Href == "" {
			b.WriteString(`<span class="crumb current">` + html.EscapeString(c.Label) + `</span>`)
		} else {
			b.WriteString(`<a class="crumb" href="` + c.Href + `">` + html.EscapeString(c.Label) + `</a>`)
		}
	}
	b.WriteString(`</nav>`)
	return b.String()
}

// wikilinkRe matches an Obsidian wikilink `[[target|label]]` (the table-cell form escapes
// the pipe as `\|`); label is optional.
var wikilinkRe = regexp.MustCompile(`\[\[([^|\]]+?)(?:\\?\|([^\]]+?))?\]\]`)

// anchorRe matches a trailing `^block` reference on a line (FR list items, glossary meta),
// capturing any list-item prefix so the anchor moves inside the item.
var anchorRe = regexp.MustCompile(`(?m)^(\s*(?:[-*] )?)(.*) \^([A-Za-z0-9_-]+)\s*$`)

// priorityRe matches the `(Priority: N - Label)` annotation the Markdown renderer puts in
// user-story headings, so the HTML can swap it for a colored badge.
var priorityRe = regexp.MustCompile(`\(Priority: (\d+) - ([^)<]+)\)`)

// htmlLinkRe matches a goldmark-rendered plain link opening tag (no attributes other than
// href), so internal cross-references can be marked with a class for styling.
var htmlLinkRe = regexp.MustCompile(`<a href="([^"]*)">`)

// styleXrefLinks tags internal cross-reference links (relative .html pages or in-page #anchors,
// i.e. everything that is not an absolute http/mailto URL) with the `xref` class so CSS can
// style them distinctly. The link text itself is already cleaned upstream (refs.renderLink).
func styleXrefLinks(body string) string {
	return htmlLinkRe.ReplaceAllStringFunc(body, func(m string) string {
		href := htmlLinkRe.FindStringSubmatch(m)[1]
		if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") || strings.HasPrefix(href, "mailto:") {
			return m
		}
		return `<a class="xref" href="` + href + `">`
	})
}

// renderContentTree renders the whole navigation tree as an expandable, styled page body —
// the index page's browsable sitemap. Top sections and domains start expanded; deeper
// sub-directories start collapsed so the page stays manageable.
func renderContentTree(nav *Nav, rel string) string {
	var b strings.Builder
	b.WriteString(`<div class="content-tree"><ul>`)
	for _, n := range nav.Root {
		b.WriteString(`<li>`)
		renderContentNode(&b, n, rel, 0)
		b.WriteString(`</li>`)
	}
	b.WriteString(`</ul></div>`)
	return b.String()
}

func renderContentNode(b *strings.Builder, n *NavNode, rel string, depth int) {
	if len(n.Children) > 0 {
		open := ""
		if depth < 2 {
			open = " open"
		}
		b.WriteString(`<details class="nav-group"` + open + `><summary>`)
		if n.Href != "" {
			navLink(b, rel+n.Href, n.Label, false, "tree-link", n.Kind)
		} else {
			navDir(b, n.Label, n.Kind)
		}
		b.WriteString(`</summary><ul>`)
		for _, c := range n.Children {
			b.WriteString(`<li>`)
			renderContentNode(b, c, rel, depth+1)
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ul></details>`)
		return
	}
	if n.Href != "" {
		navLink(b, rel+n.Href, n.Label, false, "tree-link", n.Kind)
	} else {
		navDir(b, n.Label, n.Kind)
	}
}

// decoratePriorities replaces the plain `(Priority: N - Label)` text in story headings with a
// level-colored badge (a flag icon + label). It runs on the rendered HTML, so the Markdown
// output (the Obsidian view) is left untouched. The captured label is already HTML-escaped by
// the Markdown→HTML conversion, so it is inserted verbatim.
func decoratePriorities(htmlBody string) string {
	return priorityRe.ReplaceAllStringFunc(htmlBody, func(s string) string {
		m := priorityRe.FindStringSubmatch(s)
		level, label := m[1], strings.TrimSpace(m[2])
		return `<span class="prio prio-` + level + `" title="Priority ` + level + `">` +
			icon("flag") + label + `</span>`
	})
}

// parseFrontmatter reads the leading `---`-delimited YAML block into a flat key→value map
// (values trimmed). Returns an empty map when there is no frontmatter (e.g. index pages).
func parseFrontmatter(md string) map[string]string {
	fm := map[string]string{}
	if !strings.HasPrefix(md, "---\n") {
		return fm
	}
	end := strings.Index(md[4:], "\n---")
	if end < 0 {
		return fm
	}
	for _, ln := range strings.Split(md[4:4+end], "\n") {
		if i := strings.IndexByte(ln, ':'); i > 0 {
			if k := strings.TrimSpace(ln[:i]); k != "" {
				fm[k] = strings.TrimSpace(ln[i+1:])
			}
		}
	}
	return fm
}

// renderMetaBar turns a document's frontmatter into a row of metadata chips — id, a colored
// status badge, the created date, and the (linked) domain. Returns "" when there is nothing
// to show, so non-spec pages get no bar.
func renderMetaBar(fm map[string]string, nav *Nav, rel string) string {
	var chips []string
	if id := fm["id"]; id != "" {
		chips = append(chips, `<span class="meta-chip meta-id">`+icon("hash")+`<code>`+html.EscapeString(id)+`</code></span>`)
	}
	if st := fm["status"]; st != "" {
		cls := "status-" + strings.ToLower(st)
		chips = append(chips, `<span class="meta-chip status `+cls+`">`+html.EscapeString(st)+`</span>`)
	}
	if cr := fm["created"]; cr != "" {
		chips = append(chips, `<span class="meta-chip" title="Created">`+icon("calendar")+html.EscapeString(cr)+`</span>`)
	}
	if dm := fm["domain"]; dm != "" {
		name := dm
		if nav != nil {
			name = nav.SegLabel(dm, dm) // domain slug → display name
		}
		chips = append(chips, `<a class="meta-chip" href="`+rel+dm+`/index.html">`+icon("domain")+html.EscapeString(name)+`</a>`)
	}
	if len(chips) == 0 {
		return ""
	}
	return `<div class="doc-meta">` + strings.Join(chips, "") + `</div>`
}

// injectMetaBar places the metadata bar immediately after the document's H1 (or at the top
// if there is none).
func injectMetaBar(body, meta string) string {
	if meta == "" {
		return body
	}
	const tag = "</h1>"
	if i := strings.Index(body, tag); i >= 0 {
		return body[:i+len(tag)] + "\n" + meta + body[i+len(tag):]
	}
	return meta + body
}

// pageKind picks the type-icon for a page from its path and document kind.
func pageKind(mdPath, fileKind string) string {
	switch {
	case mdPath == "index.md":
		return "specs"
	case mdPath == "entities/index.md":
		return "entities"
	case mdPath == "glossary.md":
		return "glossary"
	case mdPath == "planning/index.md":
		return "planning"
	case mdPath == "planning/capabilities.md":
		return "capability"
	case mdPath == "planning/deliverables.md":
		return "deliverable"
	case mdPath == "planning/views.md":
		return "view"
	case strings.HasSuffix(mdPath, "/index.md") && strings.Count(mdPath, "/") == 1:
		return "domain"
	case fileKind == "entity":
		return "entity"
	default:
		return "spec"
	}
}

// obsidianToGFM rewrites our Obsidian Markdown into CommonMark+GFM:
//   - YAML frontmatter is dropped (the H1 carries the title);
//   - `[[path#^anchor|label]]` → `[label](<rel>path.html#anchor)` (relative to ownerPath),
//     same-file `[[#^anchor|label]]` → `[label](#anchor)`;
//   - a trailing `^anchor` → an inline `<a id="anchor"></a>` link target.
func obsidianToGFM(md, ownerPath string) string {
	md = stripFrontmatter(md)
	rel := relPrefix(ownerPath)
	md = wikilinkRe.ReplaceAllStringFunc(md, func(s string) string {
		mm := wikilinkRe.FindStringSubmatch(s)
		target, label := mm[1], mm[2]
		if label == "" {
			label = target
		}
		return "[" + label + "](" + wikiHref(target, rel) + ")"
	})
	md = anchorRe.ReplaceAllString(md, `$1<a id="$3"></a>$2`)
	return md
}

// wikiHref turns a wikilink target into an href. rel is the `../`-prefix back to the vault
// root for cross-document links.
func wikiHref(target, rel string) string {
	switch {
	case strings.HasPrefix(target, "#^"):
		return "#" + target[2:] // same-file block reference
	case strings.HasPrefix(target, "#"):
		return target // same-file heading
	}
	pathPart, anchor := target, ""
	if i := strings.Index(target, "#^"); i >= 0 {
		pathPart, anchor = target[:i], "#"+target[i+2:]
	} else if i := strings.IndexByte(target, '#'); i >= 0 {
		pathPart, anchor = target[:i], target[i:]
	}
	return rel + pathPart + ".html" + anchor
}

// relPrefix is the `../` sequence from a document back to the vault root.
func relPrefix(ownerPath string) string {
	dir := path.Dir(ownerPath)
	if dir == "." || dir == "" {
		return ""
	}
	return strings.Repeat("../", strings.Count(dir, "/")+1)
}

func stripFrontmatter(md string) string {
	if strings.HasPrefix(md, "---\n") {
		if i := strings.Index(md[4:], "\n---\n"); i >= 0 {
			return md[4+i+5:]
		}
	}
	return md
}

// docTitle is the first H1's text (for the page <title>).
func docTitle(md string) string {
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(line[2:])
		}
	}
	return "Cusp"
}

// pageChrome is the per-page input to the HTML skeleton.
type pageChrome struct {
	Title       string
	Body        string // rendered document HTML
	Rel         string // ../-prefix back to the site root
	Sidebar     string // navigation tree HTML
	Breadcrumbs string // breadcrumb trail HTML
	PageIcon    string // type icon for this page (shown in the top bar)
}

// htmlPage wraps a rendered document in the site shell: linked stylesheet, sidebar, a sticky
// top bar with breadcrumbs and a CSS-only nav toggle for narrow screens, and a footer.
func htmlPage(p pageChrome) string {
	return "<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n" +
		"<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n" +
		"<title>" + html.EscapeString(p.Title) + " · Cusp</title>\n" +
		"<link rel=\"stylesheet\" href=\"" + p.Rel + assetStylePath + "\">\n" +
		"</head>\n<body>\n" +
		"<input type=\"checkbox\" id=\"nav-toggle\" class=\"nav-toggle\">\n" +
		"<div class=\"layout\">\n" +
		"<aside class=\"sidebar\">\n" +
		"<div class=\"brand\"><a href=\"" + p.Rel + "index.html\">Cusp</a><span class=\"brand-sub\">specs</span></div>\n" +
		p.Sidebar + "\n" +
		"</aside>\n" +
		"<main class=\"content\">\n" +
		"<header class=\"topbar\">" +
		"<label for=\"nav-toggle\" class=\"nav-toggle-btn\" aria-label=\"Toggle navigation\">☰</label>" +
		"<span class=\"page-type\">" + p.PageIcon + "</span>" +
		p.Breadcrumbs + "</header>\n" +
		"<article class=\"doc\">\n" + p.Body + "</article>\n" +
		"<footer class=\"docfoot\">Generated by Cusp — do not edit; change the database and regenerate.</footer>\n" +
		"</main>\n</div>\n</body>\n</html>"
}
