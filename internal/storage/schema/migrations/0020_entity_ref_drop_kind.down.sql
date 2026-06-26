-- 0020_entity_ref_drop_kind.down.sql — restore the kind column (always 'references').
ALTER TABLE `req_entity_ref` DROP INDEX `uk_entity_ref_identity`;
ALTER TABLE `req_entity_ref` ADD COLUMN `kind` VARCHAR(32) NOT NULL DEFAULT 'references';
ALTER TABLE `req_entity_ref`
    ADD UNIQUE KEY `uk_entity_ref_identity` (`owner_type`, `owner_id`, `target_type`, `target_id`, `kind`);
