package generate

// styleCSS is the default stylesheet shipped with the HTML output: a clean, document-oriented
// theme (StrictDoc-like) with an always-visible sidebar, breadcrumbs, and readable typography.
// It is written once per build to assets/style.css and linked by every page (depth-relative).
// No external fonts or assets — it renders the same offline.
const styleCSS = `:root {
  --bg: #ffffff;
  --bg-sidebar: #f6f8fa;
  --bg-code: #f6f8fa;
  --bg-active: #ddf4ff;
  --fg: #1f2328;
  --fg-muted: #59636e;
  --border: #d1d9e0;
  --border-muted: #e4e8ec;
  --accent: #0969da;
  --accent-active: #0550ae;
  --sidebar-w: 18rem;
  --content-w: 54rem;
  --radius: 6px;
}

* { box-sizing: border-box; }

html { -webkit-text-size-adjust: 100%; }

body {
  margin: 0;
  background: var(--bg);
  color: var(--fg);
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans", Helvetica, Arial, sans-serif;
  font-size: 16px;
  line-height: 1.6;
}

a { color: var(--accent); text-decoration: none; }
a:hover { text-decoration: underline; }

.layout { display: flex; align-items: stretch; min-height: 100vh; }

/* ---- sidebar -------------------------------------------------------------- */
.sidebar {
  width: var(--sidebar-w);
  flex: 0 0 var(--sidebar-w);
  background: var(--bg-sidebar);
  border-right: 1px solid var(--border);
  position: sticky;
  top: 0;
  align-self: flex-start;
  height: 100vh;
  overflow-y: auto;
  padding: 1.25rem 0.75rem 2rem;
}

.brand {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  padding: 0 0.5rem 1rem;
  margin-bottom: 0.5rem;
  border-bottom: 1px solid var(--border-muted);
}
.brand a {
  font-weight: 700;
  font-size: 1.15rem;
  letter-spacing: 0.02em;
  color: var(--fg);
}
.brand-sub { color: var(--fg-muted); font-size: 0.8rem; }

.nav-tree ul { list-style: none; margin: 0; padding: 0; }
.nav-tree .nav-root > li { margin-top: 0.35rem; }
/* nested levels: indent and add a guide rail */
.nav-tree ul ul { margin-left: 0.55rem; padding-left: 0.35rem; border-left: 1px solid var(--border-muted); }

.nav-tree a, .nav-tree .nav-dir {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.2rem 0.4rem;
  border-radius: var(--radius);
  color: var(--fg);
  line-height: 1.35;
  font-size: 0.86rem;
}
.nav-tree a { color: var(--fg-muted); }
.nav-tree a:hover { background: var(--border-muted); color: var(--fg); text-decoration: none; }
.nav-tree .nav-label { flex: 1 1 auto; min-width: 0; }
.nav-tree .icon { opacity: 0.8; }

/* collapsible directory groups */
.nav-group { margin: 0; }
.nav-group > summary {
  display: flex;
  align-items: center;
  gap: 0.15rem;
  list-style: none;
  cursor: pointer;
  border-radius: var(--radius);
}
.nav-group > summary::-webkit-details-marker { display: none; }
.nav-group > summary::before {
  content: "";
  flex: 0 0 auto;
  width: 0;
  height: 0;
  margin: 0 0.15rem 0 0.2rem;
  border-left: 5px solid var(--fg-muted);
  border-top: 4px solid transparent;
  border-bottom: 4px solid transparent;
  transition: transform 0.12s ease;
}
.nav-group[open] > summary::before { transform: rotate(90deg); }
.nav-group > summary:hover { background: var(--border-muted); }
.nav-group > summary > a, .nav-group > summary > .nav-dir { flex: 1 1 auto; }

/* every group header (Specifications, Entities, domains, sub-dirs) reads heavier than a leaf */
.nav-group > summary > .nav-self,
.nav-group > summary > .nav-dir {
  font-weight: 600;
  color: var(--fg);
}
/* the top-level sections (Specifications, Entities, …) stand out a touch more */
.nav-root > li > .nav-group > summary > .nav-self,
.nav-root > li > .nav-group > summary > .nav-dir,
.nav-root > li > .nav-leaf,
.nav-root > li > .nav-dir {
  font-size: 0.95rem;
}

.nav-dir { color: var(--fg-muted); }

.nav-tree a.active {
  background: var(--bg-active);
  color: var(--accent-active) !important;
  font-weight: 600;
  box-shadow: inset 2px 0 0 var(--accent);
}

/* ---- content -------------------------------------------------------------- */
.content {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.topbar {
  position: sticky;
  top: 0;
  z-index: 5;
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.6rem 2rem;
  background: rgba(255, 255, 255, 0.85);
  backdrop-filter: saturate(180%) blur(6px);
  border-bottom: 1px solid var(--border-muted);
}

.breadcrumbs { font-size: 0.85rem; color: var(--fg-muted); }
.breadcrumbs .crumb { color: var(--fg-muted); }
.breadcrumbs a.crumb:hover { color: var(--accent); }
.breadcrumbs .crumb.current { color: var(--fg); font-weight: 600; }
.breadcrumbs .sep { margin: 0 0.45rem; color: var(--border); }

.doc {
  width: 100%;
  max-width: var(--content-w);
  margin: 0 auto;
  padding: 1.5rem 2rem 4rem;
}

.doc h1, .doc h2, .doc h3, .doc h4 { line-height: 1.25; font-weight: 600; }
.doc h1 { font-size: 1.9rem; margin: 0.2rem 0 1rem; padding-bottom: 0.3rem; border-bottom: 1px solid var(--border-muted); }
.doc h2 { font-size: 1.4rem; margin: 2rem 0 0.8rem; padding-bottom: 0.25rem; border-bottom: 1px solid var(--border-muted); }
.doc h3 { font-size: 1.15rem; margin: 1.6rem 0 0.6rem; }
.doc h4 { font-size: 1rem; margin: 1.3rem 0 0.5rem; color: var(--fg-muted); }

.doc p, .doc ul, .doc ol { margin: 0.6rem 0; }
.doc li { margin: 0.2rem 0; }

.doc code {
  font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
  font-size: 0.88em;
  background: var(--bg-code);
  padding: 0.15em 0.4em;
  border-radius: var(--radius);
}
.doc pre {
  background: var(--bg-code);
  padding: 0.9rem 1rem;
  border-radius: var(--radius);
  overflow-x: auto;
  border: 1px solid var(--border-muted);
}
.doc pre code { background: none; padding: 0; font-size: 0.85rem; }

.doc blockquote {
  margin: 0.8rem 0;
  padding: 0.2rem 1rem;
  color: var(--fg-muted);
  border-left: 3px solid var(--border);
}

.doc table {
  border-collapse: collapse;
  width: 100%;
  margin: 1rem 0;
  font-size: 0.92rem;
}
.doc th, .doc td { border: 1px solid var(--border); padding: 0.45rem 0.7rem; text-align: left; vertical-align: top; }
.doc th { background: var(--bg-sidebar); font-weight: 600; }
.doc tr:nth-child(even) td { background: #fbfcfd; }

.doc hr { border: none; border-top: 1px solid var(--border-muted); margin: 2rem 0; }

.doc a[id] { scroll-margin-top: 4rem; }

.docfoot {
  max-width: var(--content-w);
  margin: 0 auto;
  padding: 1.5rem 2rem 3rem;
  color: var(--fg-muted);
  font-size: 0.8rem;
  border-top: 1px solid var(--border-muted);
  width: 100%;
}

/* ---- iconography ---------------------------------------------------------- */
.icon {
  width: 1em;
  height: 1em;
  flex: 0 0 auto;
  vertical-align: -0.15em;
}
.page-type { display: inline-flex; align-items: center; color: var(--fg-muted); }
.page-type .icon { width: 1.05rem; height: 1.05rem; }

/* user-story priority badges (0 Critical → 4 Backlog) */
.prio {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  margin-left: 0.5rem;
  padding: 0.05rem 0.5rem;
  border-radius: 999px;
  border: 1px solid transparent;
  font-size: 0.72rem;
  font-weight: 600;
  letter-spacing: 0.01em;
  vertical-align: 0.12em;
  white-space: nowrap;
}
.prio .icon { width: 0.85em; height: 0.85em; }
.prio-0 { background: #ffebe9; color: #cf222e; border-color: #ffcecb; }
.prio-1 { background: #fff1e5; color: #bc4c00; border-color: #ffd8b5; }
.prio-2 { background: #ddf4ff; color: #0969da; border-color: #b6e3ff; }
.prio-3 { background: #eef1f4; color: #59636e; border-color: #d1d9e0; }
.prio-4 { background: #f6f8fa; color: #8c959f; border-color: #e4e8ec; }

/* spec metadata bar (under the H1): id, status, created, domain */
.doc-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin: 0.75rem 0 1.5rem;
}
.meta-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.15rem 0.6rem;
  border-radius: 999px;
  border: 1px solid var(--border);
  background: var(--bg-sidebar);
  color: var(--fg-muted);
  font-size: 0.8rem;
  font-weight: 500;
  text-decoration: none;
  white-space: nowrap;
}
.meta-chip .icon { width: 0.9em; height: 0.9em; opacity: 0.85; }
a.meta-chip:hover { border-color: var(--accent); color: var(--accent); }
.meta-id code { background: none; padding: 0; font-weight: 600; color: var(--fg); }

.meta-chip.status { font-weight: 600; border-color: transparent; }
.status-draft    { background: #eef1f4; color: #59636e; border-color: #d1d9e0; }
.status-reviewed { background: #ddf4ff; color: #0969da; border-color: #b6e3ff; }
.status-active   { background: #dafbe1; color: #1a7f37; border-color: #aceebb; }
.status-obsolete { background: #f6f8fa; color: #8c959f; border-color: #e4e8ec; text-decoration: line-through; }

/* internal cross-reference links (resolved [[TYPE:key]]) — a subtle accent pill */
.doc a.xref {
  color: var(--accent-active);
  text-decoration: none;
  background: var(--bg-active);
  padding: 0.02em 0.3em;
  border-radius: 4px;
  font-weight: 500;
  white-space: nowrap;
}
.doc a.xref:hover { text-decoration: underline; }

/* index page browsable tree (sitemap) */
.lede { color: var(--fg-muted); font-size: 1.05rem; margin: 0.25rem 0 1.5rem; }
.content-tree ul { list-style: none; margin: 0; padding: 0; }
.content-tree > ul > li { margin-top: 0.35rem; }
.content-tree ul ul { margin-left: 0.75rem; padding-left: 0.5rem; border-left: 1px solid var(--border-muted); }
.content-tree a, .content-tree .nav-dir {
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
  padding: 0.22rem 0.4rem;
  border-radius: var(--radius);
  color: var(--fg);
  text-decoration: none;
}
.content-tree a:hover { background: var(--bg-active); color: var(--accent-active); }
.content-tree .nav-dir { color: var(--fg-muted); }
.content-tree .icon { width: 1.05em; height: 1.05em; opacity: 0.8; }
.content-tree summary > .tree-link,
.content-tree summary > .nav-dir { font-weight: 600; }
.content-tree > ul > li > .nav-group > summary > .tree-link,
.content-tree > ul > li > .nav-group > summary > .nav-dir { font-weight: 700; font-size: 1.05rem; }

/* ---- responsive / CSS-only nav toggle ------------------------------------- */
.nav-toggle { display: none; }
.nav-toggle-btn {
  display: none;
  cursor: pointer;
  font-size: 1.2rem;
  line-height: 1;
  padding: 0.15rem 0.5rem;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--fg-muted);
  user-select: none;
}

@media (max-width: 60rem) {
  .sidebar {
    position: fixed;
    z-index: 20;
    left: 0;
    top: 0;
    transform: translateX(-100%);
    transition: transform 0.18s ease;
    box-shadow: 0 0 1.5rem rgba(0, 0, 0, 0.18);
  }
  .nav-toggle:checked ~ .layout .sidebar { transform: translateX(0); }
  .nav-toggle-btn { display: inline-block; }
}
`
