-- 0008_glossary_term.up.sql — shared project vocabulary. A GlossaryTerm defines a
-- concept once (the `slug` is its [[TERM:slug]] cross-reference link key) so humans
-- and agents reference it everywhere instead of repeating the definition. Distinct
-- from the business `entity` layer (documents) — this is project vocabulary. A term's
-- `definition` may itself contain inline [[..]] links (a term is an entity_ref owner).
-- GlossaryAlias maps alternate surface forms to one term (globally UNIQUE alias).
-- See docs/entities/glossary.md and docs/entities/decisions.md.
CREATE TABLE IF NOT EXISTS `glossary_term` (
    `id`         VARCHAR(255) NOT NULL,
    `slug`       VARCHAR(255) NOT NULL,                  -- [[TERM:slug]] link key, kebab-case
    `term`       VARCHAR(512),                           -- display name
    `definition` TEXT,                                   -- may contain inline [[..]] links
    `domain_id`  VARCHAR(255),                           -- optional scoping (nullable); null = global
    `status`     VARCHAR(32)  NOT NULL DEFAULT 'draft',  -- values: draft|active|deprecated
    `created_at` DATETIME,
    `updated_at` DATETIME,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_glossary_term_slug` (`slug`),
    INDEX `idx_glossary_term_domain` (`domain_id`)
);

CREATE TABLE IF NOT EXISTS `glossary_alias` (
    `id`      VARCHAR(255) NOT NULL,
    `term_id` VARCHAR(255) NOT NULL,
    `alias`   VARCHAR(255) NOT NULL,                     -- alternate surface form
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_glossary_alias` (`alias`),
    INDEX `idx_glossary_alias_term` (`term_id`)
);
