-- 0005_spec_more_info.down.sql — revert the FR-area catch-all column.
ALTER TABLE `spec` DROP COLUMN `more_info`;
