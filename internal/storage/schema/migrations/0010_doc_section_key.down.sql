-- 0010_doc_section_key.down.sql — restore the column shape (not the content; doc
-- content is git-ignored and reproducible via `cusp import`, like every prior
-- section migration). A rollback is followed by a re-import.
ALTER TABLE `doc_section`
    DROP KEY `uk_doc_section_owner_key`,
    DROP COLUMN `section_key`;

ALTER TABLE `spec`
    ADD COLUMN `preamble` TEXT,
    ADD COLUMN `overview` TEXT,
    ADD COLUMN `edge_cases` TEXT,
    ADD COLUMN `success_criteria` TEXT,
    ADD COLUMN `platform_scope` TEXT,
    ADD COLUMN `assumptions` TEXT,
    ADD COLUMN `clarifications` TEXT,
    ADD COLUMN `more_info` TEXT;

ALTER TABLE `entity`
    ADD COLUMN `purpose` TEXT,
    ADD COLUMN `key_concepts` TEXT,
    ADD COLUMN `schema_reference` TEXT,
    ADD COLUMN `relationships` TEXT,
    ADD COLUMN `business_rules` TEXT,
    ADD COLUMN `validations` TEXT,
    ADD COLUMN `row_level_access` TEXT,
    ADD COLUMN `entity_notes` TEXT,
    ADD COLUMN `spec_references` TEXT;
