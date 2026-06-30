-- 0013_section_vocabulary.down.sql — restore the req_doc_section shape (not the
-- content; section content is git-ignored and reproducible via `cusp import`, like
-- every prior section migration). A rollback is followed by a re-import. Drop the
-- instance tables (they FK the type tables) before the type tables.

DROP TABLE IF EXISTS `req_spec_section`;
DROP TABLE IF EXISTS `ent_entity_section`;
DROP TABLE IF EXISTS `req_spec_section_type`;
DROP TABLE IF EXISTS `ent_entity_section_type`;

CREATE TABLE IF NOT EXISTS `req_doc_section` (
    `id`          VARCHAR(255) NOT NULL,
    `owner_type`  VARCHAR(32)  NOT NULL,                 -- values: spec|entity
    `owner_id`    VARCHAR(255) NOT NULL,
    `ordinal`     INT          NOT NULL,
    `level`       INT          NOT NULL,
    `heading`     TEXT,
    `body`        TEXT,
    `section_key` VARCHAR(255) NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_doc_section_owner_ordinal` (`owner_type`, `owner_id`, `ordinal`),
    UNIQUE KEY `uk_doc_section_owner_key` (`owner_type`, `owner_id`, `section_key`),
    INDEX `idx_doc_section_owner` (`owner_type`, `owner_id`)
);
