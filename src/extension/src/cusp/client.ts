// The transport-agnostic contract between the extension UI and a Cusp workspace.
//
// The UI talks ONLY to this interface. Today it is implemented by shelling out
// to the `cusp` CLI (see cliClient.ts); when `cusp serve --mcp` lands it can be
// re-implemented over MCP without touching any view code — same methods, same
// shapes, different transport. Keep this file free of `vscode` and `child_process`
// imports so the boundary stays clean.

export interface Changeset {
  /** The Dolt branch holding the proposed change (the changeset's stable id). */
  branch: string;
  title: string;
  /** draft | open | changes_requested | approved | denied | merged | closed */
  status: string;
}

/** One functional requirement (FR) — a leaf of the requirements tree. */
export interface ReqNode {
  frKey: string;
  statement: string;
  deliveryStatus?: string;
  milestone?: string;
}

/** A named FR group within a spec. */
export interface GroupNode {
  title: string;
  requirements: ReqNode[];
}

/** A user story — a subitem of the spec's "User Stories" bucket. */
export interface StoryNode {
  title: string;
}

/** A prose section — a subitem of the spec's "Other" bucket. */
export interface SectionNode {
  key: string;
  title: string;
}

/** A spec, grouped into User Stories / Functional Requirements / Other. */
export interface SpecNode {
  prefix?: string;
  title: string;
  docPath: string;
  stories: StoryNode[];
  groups: GroupNode[];
  requirements: ReqNode[]; // ungrouped FRs
  sections: SectionNode[]; // prose sections → "Other"
}

/** A domain and its specs — a root of the requirements tree. */
export interface DomainNode {
  slug: string;
  name: string;
  specs: SpecNode[];
}

/** A structured relationship from an entity to another entity. */
export interface EntityRel {
  name: string; // the other entity
  docPath: string; // the other entity's doc (for navigation)
  cardinality?: string;
  outgoing: boolean; // true: this entity → other; false: other → this
}

/** A prose section of an entity. */
export interface EntitySection {
  key: string;
  title: string;
}

/** An entity — a root of the entities tree. */
export interface EntityTreeNode {
  name: string;
  docPath: string;
  relationships: EntityRel[];
  sections: EntitySection[];
}

/** A review verdict on a changeset. */
export type Verdict = "approve" | "deny" | "request_changes";

/** One column's base→head change within an entity. */
export interface FieldDiff {
  name: string;
  base: string;
  head: string;
}

/** A changeset's per-entity diff plus the exact refs to render each side at. */
export interface ChangesetDiff {
  base: string; // the merge-base commit
  head: string; // the changeset branch while live, else its recorded head/merge commit
  entities: EntityDiff[];
}

/** One entity's change in a changeset — the coordinates a comment can anchor to. */
export interface EntityDiff {
  subjectType: string; // requirement | spec | user_story | entity | deliverable | test_case
  subjectId: string;
  subjectRef: string; // display label, e.g. "requirement:ATT-FR-012"
  docRef: string; // the doc to render for a base/head diff (spec prefix); "" if none
  changeType: string; // added | modified | removed
  fields: FieldDiff[];
}

/** A review comment on a changeset (threaded via parentId, anchored via subject*). */
export interface Comment {
  id: string;
  changesetId: string;
  authorId: string;
  authorHandle: string;
  parentId?: string;
  body: string;
  subjectType?: string;
  subjectId?: string;
  subjectRef?: string;
  locator?: string;
  resolved: boolean;
  createdAt?: string;
}

/** Input for adding a comment. Provide either `subject` (TYPE:key) or subjectType+subjectId. */
export interface AddCommentInput {
  branch: string;
  body: string;
  subject?: string;
  subjectType?: string;
  subjectId?: string;
  locator?: string;
  reply?: string; // parent comment id (threaded reply)
}

export interface CuspClient {
  /** Mirrors `cusp changeset ls`. */
  listChangesets(): Promise<Changeset[]>;

  /** Mirrors `cusp req tree` — the domain → spec → group → requirement hierarchy. */
  requirementsTree(): Promise<DomainNode[]>;

  /** Mirrors `cusp entity tree` — entities with their relationships and sections. */
  entitiesTree(): Promise<EntityTreeNode[]>;

  /** Render one doc as self-contained HTML — dispatches to `cusp entity render` for entity
   *  doc paths (entities/…), else `cusp spec render`. */
  renderDocHtml(docPath: string): Promise<string>;

  /** Render one doc as raw Markdown at a specific branch (e.g. `main` for the base, the
   *  changeset branch for the head) — the two sides of a native diff editor. */
  renderDocMarkdown(docRef: string, branch: string): Promise<string>;

  /** Mirrors `cusp changeset diff <branch> --entities` — the per-entity/field diff plus the
   *  exact base/head refs to render each side of the diff editor at. */
  diff(branch: string): Promise<ChangesetDiff>;

  /** Mirrors `cusp comment ls <branch>` — the changeset's comments (threaded via parentId). */
  listComments(branch: string): Promise<Comment[]>;

  /** Mirrors `cusp comment add …` — add a comment (optionally anchored / a reply). */
  addComment(input: AddCommentInput): Promise<Comment>;

  /** Mirrors `cusp comment resolve|reopen <id>`. */
  resolveComment(id: string, resolved: boolean): Promise<void>;

  /** Mirrors `cusp review <branch> --verdict …` — set the reviewer's verdict. */
  setReview(branch: string, verdict: Verdict, summary?: string): Promise<void>;

  /** Mirrors `cusp changeset start <title>` — open a new changeset (a Dolt branch). */
  startChangeset(title: string): Promise<void>;

  /** Mirrors `cusp changeset submit <branch>` — mark a changeset open for review. */
  submitChangeset(branch: string): Promise<void>;

  /** Mirrors `cusp changeset merge <branch>` — merge a changeset into main. */
  mergeChangeset(branch: string): Promise<void>;

  /** Mirrors `cusp changeset abandon <branch>` — discard a changeset (deletes its branch). */
  abandonChangeset(branch: string): Promise<void>;
}
