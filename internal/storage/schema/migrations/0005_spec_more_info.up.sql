-- 0005_spec_more_info.up.sql — a catch-all for FR-area content that is neither a
-- requirement nor a real FR group: note-only bold sub-headers (e.g. "Column
-- Configuration", "Adjustment-Specific Fields") and their config/table bodies.
-- These were previously dropped (a bold header with no FRs under it); now they
-- collect here verbatim so the Requirements section round-trips. See
-- docs/entities/decisions.md.
ALTER TABLE `spec` ADD COLUMN `more_info` TEXT;
