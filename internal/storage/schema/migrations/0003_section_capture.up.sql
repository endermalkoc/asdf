-- 0003_section_capture.up.sql — capture every spec/entity document section so a
-- regenerate from the DB is information-complete (see docs/entities/decisions.md).
--
-- Strategy (set by the section inventory of the tutor corpus): the recurring
-- template sections become typed columns; bespoke one-off sections land in a
-- generic doc_section catch-all; Clarifications become structured rows; "Key
-- Entities" lists become spec→entity edges (no new schema). FRs/stories/scenarios
-- stay typed as before.

-- Feature-spec template sections (single instance per spec). Stored verbatim as
-- Markdown — robust to the corpus's inconsistent inner formatting (e.g.
-- Clarifications mixes "### Session" date blocks, Q/A bullets, and tables).
ALTER TABLE `spec`
    ADD COLUMN `heading`          TEXT,   -- the H1 line (e.g. "Feature Specification: Add New Student")
    ADD COLUMN `preamble`         TEXT,   -- content between the H1 and the first ## section (metadata block)
    ADD COLUMN `overview`         TEXT,
    ADD COLUMN `edge_cases`       TEXT,
    ADD COLUMN `success_criteria` TEXT,
    ADD COLUMN `platform_scope`   TEXT,
    ADD COLUMN `assumptions`      TEXT,
    ADD COLUMN `clarifications`   TEXT;

-- Per-story rationale (promoted from prose).
ALTER TABLE `user_story`
    ADD COLUMN `why_priority`     TEXT,
    ADD COLUMN `independent_test` TEXT;

-- FR grouping sub-header (e.g. "Student Section", "Family Section (Children Only)").
ALTER TABLE `requirement`
    ADD COLUMN `section` VARCHAR(255);

-- Entity-doc template sections (entity docs are rigidly templated).
ALTER TABLE `entity`
    ADD COLUMN `purpose`          TEXT,
    ADD COLUMN `key_concepts`     TEXT,
    ADD COLUMN `schema_reference` TEXT,
    ADD COLUMN `relationships`    TEXT,
    ADD COLUMN `business_rules`   TEXT,
    ADD COLUMN `validations`      TEXT,
    ADD COLUMN `row_level_access` TEXT,
    ADD COLUMN `entity_notes`     TEXT,   -- the entity doc's "## Notes" section
    ADD COLUMN `spec_references`  TEXT;

-- doc_section: the generic catch-all for any document section not modeled as a
-- typed field. owner is POLYMORPHIC (spec|entity) so it is not an FK. Ordinal
-- preserves the section's original document position.
CREATE TABLE IF NOT EXISTS `doc_section` (
    `id`         VARCHAR(255) NOT NULL,
    `owner_type` VARCHAR(32)  NOT NULL,   -- values: spec|entity
    `owner_id`   VARCHAR(255) NOT NULL,
    `ordinal`    INT          NOT NULL,   -- original position in the source doc
    `level`      INT          NOT NULL,   -- heading depth: 2 = ##, 3 = ###
    `heading`    TEXT,
    `body`       TEXT,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_doc_section_owner_ordinal` (`owner_type`, `owner_id`, `ordinal`),
    INDEX `idx_doc_section_owner` (`owner_type`, `owner_id`)
);
