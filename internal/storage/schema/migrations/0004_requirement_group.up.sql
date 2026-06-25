-- 0004_requirement_group.up.sql — make the FR group a first-class entity so the
-- Functional Requirements list round-trips exactly (see docs/entities/decisions.md).
--
-- A spec's FR list is organized under bold sub-headers ("Student Section", "Family
-- Section (Children Only)", …), often with an interspersed note (a "> See [shared/X]"
-- blockquote). Modeling the group as a row captures its order (position), header,
-- and note; requirements link to it and carry their own document position — so
-- generation reconstructs the source order and grouping instead of sorting by FR
-- number. Replaces the denormalized `requirement.section` string column.

CREATE TABLE IF NOT EXISTS `requirement_group` (
    `id`       VARCHAR(255) NOT NULL,
    `spec_id`  VARCHAR(255) NOT NULL,
    `position` INT          NOT NULL,            -- order of the group within the spec's FR list
    `header`   VARCHAR(512) NOT NULL,            -- the bold sub-header text
    `note`     TEXT,                             -- interspersed prose (e.g. a "> See [shared/X]" blockquote)
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_requirement_group_spec_header` (`spec_id`, `header`),
    INDEX `idx_requirement_group_spec` (`spec_id`),
    CONSTRAINT `fk_requirement_group_spec` FOREIGN KEY (`spec_id`) REFERENCES `spec` (`id`) ON DELETE CASCADE
);

ALTER TABLE `requirement`
    ADD COLUMN `group_id` VARCHAR(255),          -- FK → requirement_group; null for ungrouped FRs
    ADD COLUMN `position` INT;                   -- document order within the spec's FR list

ALTER TABLE `requirement`
    ADD INDEX `idx_requirement_group` (`group_id`),
    ADD CONSTRAINT `fk_requirement_group` FOREIGN KEY (`group_id`) REFERENCES `requirement_group` (`id`) ON DELETE SET NULL;

ALTER TABLE `requirement` DROP COLUMN `section`;
