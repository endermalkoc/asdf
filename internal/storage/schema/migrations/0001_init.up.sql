-- 0001_init.up.sql — initial Cusp schema (all layers).
--
-- Source of truth: docs/entities/. This migration is generated from those entity
-- docs; keep them in sync (CLAUDE.md). Conventions (matching the Dolt-proven
-- beads schema, and Cusp's invariants):
--   * Every `id` is VARCHAR(255): holds a ULID (authored rows) or a deterministic
--     id (pure-relationship rows) — see docs/entities/identifiers.md. Minted by the
--     app, never DB-generated.
--   * Enums are VARCHAR, not SQL ENUM — values listed in `-- values:` comments and
--     enforced in the app, so the value set stays configurable (CLAUDE.md #4) and
--     merges/ALTERs stay painless. Canonical list: docs/entities/enums.md.
--   * Concrete FKs are enforced; POLYMORPHIC links (Edge, ExternalRef, Comment
--     anchor) cannot be FKs (they target multiple tables) — enforced in the app.
--   * Pure-relationship tables: junctions use a composite PK; `edge`/`test_result`
--     keep a surrogate `id` DERIVED deterministically from their UNIQUE identity.

-- ============================================================================
-- Structure layer
-- ============================================================================

CREATE TABLE IF NOT EXISTS `domain` (
    `id`           VARCHAR(36) NOT NULL,
    `abbreviation` VARCHAR(64)  NOT NULL,
    `name`         VARCHAR(255) NOT NULL,
    `status`       VARCHAR(32)  NOT NULL DEFAULT 'draft', -- values: draft|active|deprecated
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_domain_abbreviation` (`abbreviation`)
);

CREATE TABLE IF NOT EXISTS `spec` (
    `id`         VARCHAR(36)  NOT NULL,
    `domain_id`  VARCHAR(36)  NOT NULL,
    `prefix`     VARCHAR(6),                              -- nullable for FR-exempt docs
    `slug`       VARCHAR(255),
    `path`       VARCHAR(1024),                            -- directory only (domain-relative, no filename); NULL = top-level. Filename = slug + ".md"
    `title`      VARCHAR(512),
    `status`     VARCHAR(32)   NOT NULL DEFAULT 'draft',  -- values: draft|reviewed|active|obsolete
    `created_at` DATETIME,
    `updated_at` DATETIME,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_spec_prefix` (`prefix`),
    UNIQUE KEY `uk_spec_path` (`path`),
    INDEX `idx_spec_domain` (`domain_id`),
    CONSTRAINT `fk_spec_domain` FOREIGN KEY (`domain_id`) REFERENCES `domain` (`id`)
);

-- ============================================================================
-- Requirements layer
-- ============================================================================

CREATE TABLE IF NOT EXISTS `milestone` (
    `id`           VARCHAR(36) NOT NULL,
    `abbreviation` VARCHAR(64)  NOT NULL,                  -- seed values e.g. M0..M7, Future
    `name`         VARCHAR(255),
    `description`  TEXT,
    `sequence`     INT,
    `status`       VARCHAR(32)  NOT NULL DEFAULT 'pending', -- values: complete|in_progress|pending
    `created_at`  DATETIME,
    `updated_at`  DATETIME,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_milestone_abbreviation` (`abbreviation`)
);

CREATE TABLE IF NOT EXISTS `user_story` (
    `id`       VARCHAR(36) NOT NULL,
    `spec_id`  VARCHAR(36) NOT NULL,
    `ordinal`  INT          NOT NULL,                      -- unique within spec (heading number)
    `title`    VARCHAR(512),
    `priority` VARCHAR(8),                                 -- legacy P1|P2|P3; migration 0018 converts to INT level (0–4, req_priority)
    `as_a`     TEXT,
    `i_want`   TEXT,
    `so_that`  TEXT,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_user_story_spec_ordinal` (`spec_id`, `ordinal`),
    INDEX `idx_user_story_spec` (`spec_id`),
    CONSTRAINT `fk_user_story_spec` FOREIGN KEY (`spec_id`) REFERENCES `spec` (`id`) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS `acceptance_scenario` (
    `id`            VARCHAR(36) NOT NULL,
    `user_story_id` VARCHAR(36) NOT NULL,
    `ordinal`       INT          NOT NULL,                 -- order within story
    `given`         TEXT,
    `when`          TEXT,
    `then`          TEXT,
    PRIMARY KEY (`id`),
    INDEX `idx_acceptance_scenario_story` (`user_story_id`),
    CONSTRAINT `fk_acceptance_scenario_story` FOREIGN KEY (`user_story_id`) REFERENCES `user_story` (`id`) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS `requirement` (
    `id`              VARCHAR(36) NOT NULL,
    `spec_id`         VARCHAR(36) NOT NULL,               -- owns the numbering namespace
    `number`          INT          NOT NULL,               -- sequential within spec
    `suffix`          CHAR(1),                             -- optional sub-letter (a,b,...)
    `parent_id`       VARCHAR(36),                        -- set on sub-requirements
    `fr_key`          VARCHAR(64),                         -- derived citation form (e.g. ATT-FR-012); app-maintained, see note below
    `statement`       TEXT,
    `content_status`  VARCHAR(32)  NOT NULL DEFAULT 'draft', -- values: draft|active|obsolete
    `delivery_status` VARCHAR(32),                         -- values: covered|test-pending|not-implemented|e2e-sufficient|shared|schema-only|deferred
    `milestone_id`    VARCHAR(36),
    `notes`           TEXT,
    `tombstoned_at`   DATE,
    `created_at`     DATETIME,
    `updated_at`     DATETIME,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_requirement_spec_number_suffix` (`spec_id`, `number`, `suffix`),
    INDEX `idx_requirement_spec` (`spec_id`),
    INDEX `idx_requirement_milestone` (`milestone_id`),
    INDEX `idx_requirement_parent` (`parent_id`),
    INDEX `idx_requirement_fr_key` (`fr_key`),
    CONSTRAINT `fk_requirement_spec` FOREIGN KEY (`spec_id`) REFERENCES `spec` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_requirement_milestone` FOREIGN KEY (`milestone_id`) REFERENCES `milestone` (`id`) ON DELETE SET NULL,
    CONSTRAINT `fk_requirement_parent` FOREIGN KEY (`parent_id`) REFERENCES `requirement` (`id`) ON DELETE SET NULL
);
-- NOTE on fr_key: identifiers.md specifies fr_key = spec.prefix || '-FR-' || lpad(number,3,'0') || coalesce(suffix,'').
-- A SQL generated column cannot reference another table (spec.prefix), so fr_key is a plain
-- column maintained by the store on write, not a STORED/VIRTUAL generated column. The real
-- DB-enforced business key is UNIQUE(spec_id, number, suffix) above.

-- ============================================================================
-- Testing layer (Qase-style)
-- ============================================================================

CREATE TABLE IF NOT EXISTS `test_suite` (
    `id`          VARCHAR(36) NOT NULL,
    `parent_id`   VARCHAR(36),                            -- self-ref; null at root
    `name`        VARCHAR(255),
    `description` TEXT,
    `position`    INT,
    PRIMARY KEY (`id`),
    INDEX `idx_test_suite_parent` (`parent_id`),
    CONSTRAINT `fk_test_suite_parent` FOREIGN KEY (`parent_id`) REFERENCES `test_suite` (`id`) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS `test_case` (
    `id`            VARCHAR(36) NOT NULL,
    `suite_id`      VARCHAR(36) NOT NULL,
    `title`         VARCHAR(512),
    `description`   TEXT,
    `preconditions` TEXT,
    `layer`         VARCHAR(32),                           -- values: unit|integration|e2e|component|shared
    `type`          VARCHAR(32),                           -- values: functional|smoke|regression|acceptance|other
    `priority`      VARCHAR(16),                           -- legacy low|medium|high; migration 0018 converts to INT level (0–4, req_priority)
    `severity`      VARCHAR(16),                           -- values: trivial|minor|normal|major|critical|blocker
    `automation`    VARCHAR(32),                           -- values: manual|automated|to_be_automated
    `status`        VARCHAR(32)  NOT NULL DEFAULT 'draft', -- values: draft|active|deprecated (lifecycle, not run outcome)
    `path`          VARCHAR(1024),                         -- automated test file; nullable
    `is_flaky`      BOOLEAN      NOT NULL DEFAULT FALSE,
    `created_at`   DATETIME,
    `updated_at`   DATETIME,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_test_case_suite_title` (`suite_id`, `title`),
    INDEX `idx_test_case_suite` (`suite_id`),
    CONSTRAINT `fk_test_case_suite` FOREIGN KEY (`suite_id`) REFERENCES `test_suite` (`id`) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS `test_step` (
    `id`              VARCHAR(36) NOT NULL,
    `test_case_id`    VARCHAR(36) NOT NULL,
    `ordinal`         INT          NOT NULL,
    `action`          TEXT,
    `expected_result` TEXT,
    PRIMARY KEY (`id`),
    INDEX `idx_test_step_case` (`test_case_id`),
    CONSTRAINT `fk_test_step_case` FOREIGN KEY (`test_case_id`) REFERENCES `test_case` (`id`) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS `test_run` (
    `id`           VARCHAR(36) NOT NULL,
    `title`        VARCHAR(512),
    `description`  TEXT,
    `status`       VARCHAR(32)  NOT NULL DEFAULT 'active', -- values: active|complete|aborted
    `milestone_id` VARCHAR(36),
    `started_at`   DATETIME,
    `ended_at`     DATETIME,
    PRIMARY KEY (`id`),
    INDEX `idx_test_run_milestone` (`milestone_id`),
    CONSTRAINT `fk_test_run_milestone` FOREIGN KEY (`milestone_id`) REFERENCES `milestone` (`id`) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS `configuration` (
    `id`          VARCHAR(36) NOT NULL,
    `group`       VARCHAR(255) NOT NULL,                   -- e.g. Browser, OS, Environment
    `name`        VARCHAR(255) NOT NULL,                   -- the value, e.g. Chrome
    `description` TEXT,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_configuration_group_name` (`group`, `name`)
);

-- test_result: pure-relationship row. `id` is DERIVED deterministically from
-- UNIQUE(run_id, test_case_id, configuration_id) (identifiers.md) — not a random ULID.
CREATE TABLE IF NOT EXISTS `test_result` (
    `id`               VARCHAR(36) NOT NULL,              -- deterministic over the UNIQUE identity below
    `run_id`           VARCHAR(36) NOT NULL,
    `test_case_id`     VARCHAR(36) NOT NULL,
    `configuration_id` VARCHAR(36),                       -- nullable
    `status`           VARCHAR(32),                        -- values: passed|failed|blocked|skipped|invalid|in_progress
    `comment`          TEXT,
    `duration_ms`      INT,
    `stacktrace`       TEXT,
    `executed_by`      VARCHAR(255),
    `executed_at`      DATETIME,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_test_result_identity` (`run_id`, `test_case_id`, `configuration_id`),
    INDEX `idx_test_result_run` (`run_id`),
    INDEX `idx_test_result_case` (`test_case_id`),
    CONSTRAINT `fk_test_result_run` FOREIGN KEY (`run_id`) REFERENCES `test_run` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_test_result_case` FOREIGN KEY (`test_case_id`) REFERENCES `test_case` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_test_result_configuration` FOREIGN KEY (`configuration_id`) REFERENCES `configuration` (`id`) ON DELETE SET NULL
);

-- ============================================================================
-- Planning layer
-- ============================================================================

CREATE TABLE IF NOT EXISTS `capability` (
    `id`        VARCHAR(36) NOT NULL,
    `title`     VARCHAR(512),
    `level`     VARCHAR(32),                               -- values: domain|epic|capability
    `domain_id` VARCHAR(36) NOT NULL,
    `parent_id` VARCHAR(36),                              -- self-ref
    PRIMARY KEY (`id`),
    INDEX `idx_capability_domain` (`domain_id`),
    INDEX `idx_capability_parent` (`parent_id`),
    CONSTRAINT `fk_capability_domain` FOREIGN KEY (`domain_id`) REFERENCES `domain` (`id`),
    CONSTRAINT `fk_capability_parent` FOREIGN KEY (`parent_id`) REFERENCES `capability` (`id`) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS `deliverable` (
    `id`           VARCHAR(36) NOT NULL,
    `title`        VARCHAR(512),
    `size`         VARCHAR(8),                             -- values: S|M|L|XL
    `status`       VARCHAR(32)  NOT NULL DEFAULT 'proposed', -- values: proposed|specced|wired|built|ship
    `ai_ready`     VARCHAR(8),                             -- values: yes|no|na
    `milestone_id` VARCHAR(36),
    PRIMARY KEY (`id`),
    INDEX `idx_deliverable_milestone` (`milestone_id`),
    CONSTRAINT `fk_deliverable_milestone` FOREIGN KEY (`milestone_id`) REFERENCES `milestone` (`id`) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS `view` (
    `id`        VARCHAR(36)  NOT NULL,
    `title`     VARCHAR(512),
    `route`     VARCHAR(1024),                             -- app route
    `spec_id`   VARCHAR(36),                              -- nullable: set once a spec backs the view
    `domain_id` VARCHAR(36)  NOT NULL,
    PRIMARY KEY (`id`),
    INDEX `idx_view_spec` (`spec_id`),
    INDEX `idx_view_domain` (`domain_id`),
    CONSTRAINT `fk_view_spec` FOREIGN KEY (`spec_id`) REFERENCES `spec` (`id`) ON DELETE SET NULL,
    CONSTRAINT `fk_view_domain` FOREIGN KEY (`domain_id`) REFERENCES `domain` (`id`)
);

-- ============================================================================
-- Authorization & entity layer
-- ============================================================================

-- Entity is a first-class document (not a spec, not in a domain): its prose lives in
-- ent_entity_section, its structure in ent_attribute/ent_relationship. The doc lives at
-- entities/[path/]<kebab-name>.md — `path` is just the optional sub-directory under
-- entities/ (NULL = directly under entities/, the common case); the filename is derived
-- from `name` (lower-kebab). See docs/entities/decisions.md.
CREATE TABLE IF NOT EXISTS `entity` (
    `id`          VARCHAR(36) NOT NULL,
    `path`        VARCHAR(1024),                           -- sub-directory under entities/ (NULL = top-level)
    `name`        VARCHAR(255) NOT NULL,
    `description` TEXT,
    `status`      VARCHAR(32)  NOT NULL DEFAULT 'draft',   -- values: draft|active|deprecated
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_entity_name` (`name`)
);

CREATE TABLE IF NOT EXISTS `entity_attribute` (
    `id`          VARCHAR(36) NOT NULL,
    `entity_id`   VARCHAR(36) NOT NULL,
    `name`        VARCHAR(255) NOT NULL,                   -- domain property name
    `description` TEXT,
    `category`    VARCHAR(255),                            -- doc grouping; nullable
    `is_derived`  BOOLEAN      NOT NULL DEFAULT FALSE,
    `position`    INT,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_entity_attribute_entity_name` (`entity_id`, `name`),
    INDEX `idx_entity_attribute_entity` (`entity_id`),
    CONSTRAINT `fk_entity_attribute_entity` FOREIGN KEY (`entity_id`) REFERENCES `entity` (`id`) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS `entity_relationship` (
    `id`             VARCHAR(36) NOT NULL,
    `from_entity_id` VARCHAR(36) NOT NULL,
    `to_entity_id`   VARCHAR(36) NOT NULL,
    `cardinality`    VARCHAR(32),                          -- values: one_to_one|one_to_many|many_to_many
    `junction_table` VARCHAR(255),                         -- nullable (for m2m)
    `notes`          TEXT,
    PRIMARY KEY (`id`),
    INDEX `idx_entity_relationship_from` (`from_entity_id`),
    INDEX `idx_entity_relationship_to` (`to_entity_id`),
    CONSTRAINT `fk_entity_relationship_from` FOREIGN KEY (`from_entity_id`) REFERENCES `entity` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_entity_relationship_to` FOREIGN KEY (`to_entity_id`) REFERENCES `entity` (`id`) ON DELETE CASCADE
);

-- privilege + access_rule: the authorization layer. DROPPED by migration 0012
-- (replaced by entity access_control sections); the value comments below are historical.
CREATE TABLE IF NOT EXISTS `privilege` (
    `id`       VARCHAR(36) NOT NULL,
    `resource` VARCHAR(255) NOT NULL,                      -- e.g. students
    `scope`    VARCHAR(64)  NOT NULL,                      -- configurable (tenant concept); e.g. owned|studio
    `action`   VARCHAR(32)  NOT NULL,                      -- values: view|manage
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_privilege_resource_scope_action` (`resource`, `scope`, `action`)
);

CREATE TABLE IF NOT EXISTS `access_rule` (
    `id`           VARCHAR(36) NOT NULL,
    `entity_id`    VARCHAR(36) NOT NULL,
    `privilege_id` VARCHAR(36) NOT NULL,                  -- the required triple
    `condition`    TEXT,                                   -- nullable
    `description`  TEXT,
    PRIMARY KEY (`id`),
    INDEX `idx_access_rule_entity` (`entity_id`),
    INDEX `idx_access_rule_privilege` (`privilege_id`),
    CONSTRAINT `fk_access_rule_entity` FOREIGN KEY (`entity_id`) REFERENCES `entity` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_access_rule_privilege` FOREIGN KEY (`privilege_id`) REFERENCES `privilege` (`id`)
);

-- ============================================================================
-- Cross-reference graph (Edge) — polymorphic, deterministic PK
-- ============================================================================

-- edge: pure-relationship row. `id` is DERIVED deterministically from
-- UNIQUE(from_type, from_id, to_type, to_id, kind) (identifiers.md). The from/to
-- links are POLYMORPHIC (type + id over several tables) so they are not FKs.
CREATE TABLE IF NOT EXISTS `edge` (
    `id`        VARCHAR(36) NOT NULL,                     -- deterministic over the UNIQUE identity below
    `from_type` VARCHAR(32)  NOT NULL,                     -- values: requirement|spec|user_story|entity|milestone
    `from_id`   VARCHAR(36) NOT NULL,
    `to_type`   VARCHAR(32)  NOT NULL,                     -- same domain as from_type
    `to_id`     VARCHAR(36) NOT NULL,
    `kind`      VARCHAR(32)  NOT NULL,                     -- values: references|refines|depends_on|supersedes|relates|defers_to
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_edge_identity` (`from_type`, `from_id`, `to_type`, `to_id`, `kind`),
    INDEX `idx_edge_from` (`from_type`, `from_id`),
    INDEX `idx_edge_to` (`to_type`, `to_id`)
);

-- ============================================================================
-- Interop layer (ExternalRef) — polymorphic subject
-- ============================================================================

CREATE TABLE IF NOT EXISTS `external_ref` (
    `id`           VARCHAR(36)  NOT NULL,
    `subject_type` VARCHAR(32)   NOT NULL,                 -- values: deliverable|requirement|test_result
    `subject_id`   VARCHAR(36)  NOT NULL,                 -- polymorphic (subject_type + subject_id)
    `system`       VARCHAR(64)   NOT NULL,                 -- open string: jira|rally|beads|linear|github|other|...
    `external_id`  VARCHAR(255)  NOT NULL,                 -- id/key in that system
    `url`          VARCHAR(1024),                          -- nullable
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_external_ref_subject_system` (`subject_type`, `subject_id`, `system`),
    INDEX `idx_external_ref_subject` (`subject_type`, `subject_id`)
);

-- ============================================================================
-- Review & collaboration layer
-- ============================================================================

CREATE TABLE IF NOT EXISTS `actor` (
    `id`         VARCHAR(36) NOT NULL,
    `kind`       VARCHAR(32)  NOT NULL,                    -- values: human|agent
    `name`       VARCHAR(255),
    `handle`     VARCHAR(255) NOT NULL,                    -- maps to the Dolt committer
    `agent_tool` VARCHAR(64),                              -- claude|codex|cursor|opencode|...; agents only, nullable
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_actor_handle` (`handle`)
);

CREATE TABLE IF NOT EXISTS `changeset` (
    `id`          VARCHAR(36)  NOT NULL,
    `title`       VARCHAR(512),
    `description` TEXT,
    `author_id`   VARCHAR(36)  NOT NULL,
    `status`      VARCHAR(32)   NOT NULL DEFAULT 'draft',  -- values: draft|open|changes_requested|approved|denied|merged|closed
    `branch`      VARCHAR(512)  NOT NULL,                  -- the Dolt branch holding the change
    `base_commit` VARCHAR(64),                             -- merge base
    `head_commit` VARCHAR(64),                             -- branch head under review
    `merge_commit` VARCHAR(64),                            -- set when approved & merged; nullable
    `created_at`  DATETIME,
    `updated_at`  DATETIME,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_changeset_branch` (`branch`),
    INDEX `idx_changeset_author` (`author_id`),
    CONSTRAINT `fk_changeset_author` FOREIGN KEY (`author_id`) REFERENCES `actor` (`id`)
);

CREATE TABLE IF NOT EXISTS `review` (
    `id`          VARCHAR(36) NOT NULL,
    `changeset_id` VARCHAR(36) NOT NULL,
    `reviewer_id` VARCHAR(36) NOT NULL,
    `verdict`     VARCHAR(32),                             -- values: approve|deny|request_changes
    `summary`     TEXT,
    `created_at`  DATETIME,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_review_changeset_reviewer` (`changeset_id`, `reviewer_id`),
    INDEX `idx_review_changeset` (`changeset_id`),
    INDEX `idx_review_reviewer` (`reviewer_id`),
    CONSTRAINT `fk_review_changeset` FOREIGN KEY (`changeset_id`) REFERENCES `changeset` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_review_reviewer` FOREIGN KEY (`reviewer_id`) REFERENCES `actor` (`id`)
);

-- comment: subject anchor (subject_type + subject_id) is POLYMORPHIC, so not a FK.
CREATE TABLE IF NOT EXISTS `comment` (
    `id`           VARCHAR(36) NOT NULL,
    `changeset_id`  VARCHAR(36) NOT NULL,
    `author_id`    VARCHAR(36) NOT NULL,
    `parent_id`    VARCHAR(36),                           -- self-ref for threading; nullable
    `body`         TEXT,
    `subject_type` VARCHAR(32),                            -- nullable; values: requirement|spec|user_story|test_case|entity|deliverable
    `subject_id`   VARCHAR(36),                           -- nullable; anchored node
    `locator`      VARCHAR(512),                           -- field/diff-hunk locator; nullable
    `resolved`     BOOLEAN      NOT NULL DEFAULT FALSE,
    `created_at`   DATETIME,
    `updated_at`   DATETIME,
    PRIMARY KEY (`id`),
    INDEX `idx_comment_changeset` (`changeset_id`),
    INDEX `idx_comment_author` (`author_id`),
    INDEX `idx_comment_parent` (`parent_id`),
    INDEX `idx_comment_subject` (`subject_type`, `subject_id`),
    CONSTRAINT `fk_comment_changeset` FOREIGN KEY (`changeset_id`) REFERENCES `changeset` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_comment_author` FOREIGN KEY (`author_id`) REFERENCES `actor` (`id`),
    CONSTRAINT `fk_comment_parent` FOREIGN KEY (`parent_id`) REFERENCES `comment` (`id`) ON DELETE SET NULL
);

-- ============================================================================
-- Junction tables (composite PK = identity; merge-convergent by construction)
-- ============================================================================

CREATE TABLE IF NOT EXISTS `requirement_test_case` (
    `requirement_id` VARCHAR(36) NOT NULL,
    `test_case_id`   VARCHAR(36) NOT NULL,
    PRIMARY KEY (`requirement_id`, `test_case_id`),
    INDEX `idx_rtc_test_case` (`test_case_id`),
    CONSTRAINT `fk_rtc_requirement` FOREIGN KEY (`requirement_id`) REFERENCES `requirement` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_rtc_test_case` FOREIGN KEY (`test_case_id`) REFERENCES `test_case` (`id`) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS `test_run_configuration` (
    `run_id`           VARCHAR(36) NOT NULL,
    `configuration_id` VARCHAR(36) NOT NULL,
    PRIMARY KEY (`run_id`, `configuration_id`),
    INDEX `idx_trc_configuration` (`configuration_id`),
    CONSTRAINT `fk_trc_run` FOREIGN KEY (`run_id`) REFERENCES `test_run` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_trc_configuration` FOREIGN KEY (`configuration_id`) REFERENCES `configuration` (`id`) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS `capability_milestone` (
    `capability_id` VARCHAR(36) NOT NULL,
    `milestone_id`  VARCHAR(36) NOT NULL,
    PRIMARY KEY (`capability_id`, `milestone_id`),
    INDEX `idx_cm_milestone` (`milestone_id`),
    CONSTRAINT `fk_cm_capability` FOREIGN KEY (`capability_id`) REFERENCES `capability` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_cm_milestone` FOREIGN KEY (`milestone_id`) REFERENCES `milestone` (`id`) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS `capability_deliverable` (
    `capability_id`  VARCHAR(36) NOT NULL,
    `deliverable_id` VARCHAR(36) NOT NULL,
    PRIMARY KEY (`capability_id`, `deliverable_id`),
    INDEX `idx_cd_deliverable` (`deliverable_id`),
    CONSTRAINT `fk_cd_capability` FOREIGN KEY (`capability_id`) REFERENCES `capability` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_cd_deliverable` FOREIGN KEY (`deliverable_id`) REFERENCES `deliverable` (`id`) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS `deliverable_view` (
    `deliverable_id` VARCHAR(36) NOT NULL,
    `view_id`        VARCHAR(36) NOT NULL,
    PRIMARY KEY (`deliverable_id`, `view_id`),
    INDEX `idx_dv_view` (`view_id`),
    CONSTRAINT `fk_dv_deliverable` FOREIGN KEY (`deliverable_id`) REFERENCES `deliverable` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_dv_view` FOREIGN KEY (`view_id`) REFERENCES `view` (`id`) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS `deliverable_dependency` (
    `deliverable_id` VARCHAR(36) NOT NULL,
    `blocked_by_id`  VARCHAR(36) NOT NULL,                -- both → deliverable
    PRIMARY KEY (`deliverable_id`, `blocked_by_id`),
    INDEX `idx_dd_blocked_by` (`blocked_by_id`),
    CONSTRAINT `fk_dd_deliverable` FOREIGN KEY (`deliverable_id`) REFERENCES `deliverable` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_dd_blocked_by` FOREIGN KEY (`blocked_by_id`) REFERENCES `deliverable` (`id`) ON DELETE CASCADE
);
