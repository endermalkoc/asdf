-- 0012_drop_authz_layer.down.sql
--
-- Recreate the authorization layer's table *shapes* (prefixed names, as they
-- stood after migration 0011). Content is import-derived and git-ignored, so the
-- down-migration restores structure only — re-import to repopulate, same as every
-- prior schema rollback.
CREATE TABLE IF NOT EXISTS `req_privilege` (
    `id`       VARCHAR(255) NOT NULL,
    `resource` VARCHAR(255) NOT NULL,                      -- e.g. students
    `scope`    VARCHAR(64)  NOT NULL,                      -- configurable (tenant concept); e.g. owned|studio
    `action`   VARCHAR(32)  NOT NULL,                      -- values: view|manage
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_privilege_resource_scope_action` (`resource`, `scope`, `action`)
);

CREATE TABLE IF NOT EXISTS `req_access_rule` (
    `id`           VARCHAR(255) NOT NULL,
    `entity_id`    VARCHAR(255) NOT NULL,
    `privilege_id` VARCHAR(255) NOT NULL,                  -- the required triple
    `condition`    TEXT,                                   -- nullable
    `description`  TEXT,
    PRIMARY KEY (`id`),
    INDEX `idx_access_rule_entity` (`entity_id`),
    INDEX `idx_access_rule_privilege` (`privilege_id`),
    CONSTRAINT `fk_access_rule_entity` FOREIGN KEY (`entity_id`) REFERENCES `ent_entity` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_access_rule_privilege` FOREIGN KEY (`privilege_id`) REFERENCES `req_privilege` (`id`)
);
