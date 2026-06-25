-- 0004_requirement_group.down.sql — revert first-class FR groups.
ALTER TABLE `requirement` ADD COLUMN `section` VARCHAR(255);
ALTER TABLE `requirement` DROP FOREIGN KEY `fk_requirement_group`;
ALTER TABLE `requirement` DROP COLUMN `group_id`, DROP COLUMN `position`;
DROP TABLE IF EXISTS `requirement_group`;
