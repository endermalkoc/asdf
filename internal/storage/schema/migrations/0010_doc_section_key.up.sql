-- 0010_doc_section_key.up.sql — collapse the per-section typed columns on `spec`
-- and `entity` into the generic `doc_section` table, addressed by a new
-- `section_key`. The 0003 design modeled recurring sections as ~17 typed TEXT
-- columns, which DUPLICATED doc_section and baked one corpus's doc template into the
-- core schema (against the "keep the core generic" invariant). Now every prose
-- section is one model: a recognized section carries a normalized section_key
-- (overview, edge_cases, purpose, …), a bespoke one carries section_key = NULL.
--
-- `heading` stays on `spec` (the H1 identity line, not a section); `description`
-- stays on `entity` (the glossary one-liner). Generated output is UNCHANGED — the
-- generator looks recognized sections up by key at the same canonical positions.
--
-- DDL-only: section content is import-only (store.AddSpec writes none), so a
-- re-import repopulates doc_section. Like 0004 (Requirement.section → RequirementGroup).
-- See docs/entities/decisions.md and docs/entities/structure.md#docsection.
ALTER TABLE `doc_section`
    ADD COLUMN `section_key` VARCHAR(255) NULL,          -- recognized section id; NULL = bespoke
    ADD UNIQUE KEY `uk_doc_section_owner_key` (`owner_type`, `owner_id`, `section_key`);

ALTER TABLE `spec`
    DROP COLUMN `preamble`,
    DROP COLUMN `overview`,
    DROP COLUMN `edge_cases`,
    DROP COLUMN `success_criteria`,
    DROP COLUMN `platform_scope`,
    DROP COLUMN `assumptions`,
    DROP COLUMN `clarifications`,
    DROP COLUMN `more_info`;

ALTER TABLE `entity`
    DROP COLUMN `purpose`,
    DROP COLUMN `key_concepts`,
    DROP COLUMN `schema_reference`,
    DROP COLUMN `relationships`,
    DROP COLUMN `business_rules`,
    DROP COLUMN `validations`,
    DROP COLUMN `row_level_access`,
    DROP COLUMN `entity_notes`,
    DROP COLUMN `spec_references`;
