-- 0003_section_capture.down.sql — revert section capture.
DROP TABLE IF EXISTS `doc_section`;

ALTER TABLE `entity`
    DROP COLUMN `purpose`, DROP COLUMN `key_concepts`, DROP COLUMN `schema_reference`,
    DROP COLUMN `relationships`, DROP COLUMN `business_rules`, DROP COLUMN `validations`,
    DROP COLUMN `row_level_access`, DROP COLUMN `entity_notes`, DROP COLUMN `spec_references`;

ALTER TABLE `requirement` DROP COLUMN `section`;

ALTER TABLE `user_story` DROP COLUMN `why_priority`, DROP COLUMN `independent_test`;

ALTER TABLE `spec`
    DROP COLUMN `heading`, DROP COLUMN `preamble`, DROP COLUMN `overview`,
    DROP COLUMN `edge_cases`, DROP COLUMN `success_criteria`, DROP COLUMN `platform_scope`,
    DROP COLUMN `assumptions`, DROP COLUMN `clarifications`;
