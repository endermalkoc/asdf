-- 0002_domain_description.down.sql — revert Domain.description.
ALTER TABLE `domain` DROP COLUMN `description`;
